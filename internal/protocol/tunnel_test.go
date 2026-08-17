package protocol

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestDataPreambleRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteDataPreamble(&buffer, "svc-0123456789"); err != nil {
		t.Fatal(err)
	}
	kind, err := ReadTunnelStreamKind(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if kind != TunnelStreamData {
		t.Fatalf("kind = %d, want data", kind)
	}
	id, err := ReadDataPreamble(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if id != "svc-0123456789" {
		t.Fatalf("service id = %q", id)
	}
}

func TestDataPreambleRejectsInvalidIDs(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteDataPreamble(&buffer, ""); err == nil {
		t.Fatal("empty service id was accepted")
	}
	if err := WriteDataPreamble(&buffer, strings.Repeat("x", 129)); err == nil {
		t.Fatal("oversized service id was accepted")
	}
}

func TestDialStatusRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteDialStatus(&buffer, TunnelDialFailed); err != nil {
		t.Fatal(err)
	}
	status, err := ReadDialStatus(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if status != TunnelDialFailed {
		t.Fatalf("status = %d, want failed", status)
	}
}

func TestCatalogRoundTripAndUpdates(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteControlHeader(&buffer); err != nil {
		t.Fatal(err)
	}
	first := CatalogMessage{Services: []CatalogService{
		{ID: "svc-1", Name: "web", Address: "10.0.0.5:80"},
		{ID: "svc-2", Name: "ssh", Address: "10.0.0.5:22"},
	}}
	second := CatalogMessage{Services: []CatalogService{
		{ID: "svc-1", Name: "web", Address: "10.0.0.5:80"},
	}}
	if err := WriteCatalog(&buffer, first); err != nil {
		t.Fatal(err)
	}
	if err := WriteCatalog(&buffer, second); err != nil {
		t.Fatal(err)
	}

	kind, err := ReadTunnelStreamKind(&buffer)
	if err != nil || kind != TunnelStreamControl {
		t.Fatalf("kind = %d err = %v, want control", kind, err)
	}
	received, err := ReadCatalog(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if len(received.Services) != 2 || received.Services[1].Name != "ssh" {
		t.Fatalf("first catalog = %#v", received)
	}
	received, err = ReadCatalog(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if len(received.Services) != 1 {
		t.Fatalf("second catalog = %#v", received)
	}
	if _, err := ReadCatalog(&buffer); err != io.EOF {
		t.Fatalf("end of stream error = %v, want EOF", err)
	}
}

func TestReadCatalogRejectsOversizedFrames(t *testing.T) {
	frame := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	if _, err := ReadCatalog(bytes.NewReader(frame)); err == nil {
		t.Fatal("oversized catalog frame was accepted")
	}
}
