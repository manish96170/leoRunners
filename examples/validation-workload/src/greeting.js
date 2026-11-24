export function greeting(name) {
  const trimmed = String(name ?? "").trim();
  return trimmed ? `Hello, ${trimmed}` : "Hello";
}
