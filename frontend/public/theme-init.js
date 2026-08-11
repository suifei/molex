(() => {
  try {
    const saved = localStorage.getItem("molex:theme");
    const preference = saved === "light" || saved === "dark" || saved === "system" ? saved : "system";
    const theme = preference === "system"
      ? (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")
      : preference;
    const root = document.documentElement;
    root.dataset.theme = theme;
    root.dataset.themePreference = preference;
    root.style.colorScheme = theme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute(
      "content",
      theme === "dark" ? "#111216" : "#e9edf3",
    );
  } catch {
    document.documentElement.dataset.theme = "dark";
  }
})();
