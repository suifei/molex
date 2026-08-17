package relay

import (
	"errors"
	"sync"

	"github.com/suifei/molex/internal/protocol"
)

// errDuplicateTarget reports that a different target process is already
// serving the token while its connections are still alive.
var errDuplicateTarget = errors.New("another target is already connected for this token")

// group tracks every live connection of one token. Exactly one target
// process (identified by its random instance id) may be active; its adaptive
// session pool may hold many connections. Edges are unrestricted.
type group struct {
	targetInstance string
	targets        map[*participant]struct{}
	edges          map[*participant]struct{}
}

type groupSet struct {
	mu     sync.Mutex
	groups map[string]*group
}

func newGroupSet() *groupSet {
	return &groupSet{groups: make(map[string]*group)}
}

func (s *groupSet) join(p *participant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.groups[p.tokenID]
	if entry == nil {
		entry = &group{
			targets: make(map[*participant]struct{}),
			edges:   make(map[*participant]struct{}),
		}
		s.groups[p.tokenID] = entry
	}
	if p.role == protocol.RoleEdge {
		entry.edges[p] = struct{}{}
		return nil
	}
	pruneDead(entry.targets)
	instance := p.currentMetadata().Instance
	if len(entry.targets) == 0 {
		entry.targetInstance = instance
	} else if instance == "" || instance != entry.targetInstance {
		return errDuplicateTarget
	}
	entry.targets[p] = struct{}{}
	return nil
}

func (s *groupSet) leave(p *participant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.groups[p.tokenID]
	if entry == nil {
		return
	}
	delete(entry.edges, p)
	delete(entry.targets, p)
	if len(entry.targets) == 0 {
		entry.targetInstance = ""
	}
	if len(entry.targets) == 0 && len(entry.edges) == 0 {
		delete(s.groups, p.tokenID)
	}
}

// members returns every live participant of one token so a disabled or
// deleted token can be disconnected as a group.
func (s *groupSet) members(tokenID string) []*participant {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.groups[tokenID]
	if entry == nil {
		return nil
	}
	result := make([]*participant, 0, len(entry.targets)+len(entry.edges))
	for p := range entry.targets {
		result = append(result, p)
	}
	for p := range entry.edges {
		result = append(result, p)
	}
	return result
}

func (s *groupSet) tokenIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.groups))
	for id := range s.groups {
		ids = append(ids, id)
	}
	return ids
}

func pruneDead(participants map[*participant]struct{}) {
	for p := range participants {
		if p.closed() {
			delete(participants, p)
		}
	}
}
