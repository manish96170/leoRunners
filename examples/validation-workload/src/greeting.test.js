import { expect, test } from "vitest";
import { greeting } from "./greeting.js";

test("greets by name", () => {
  expect(greeting("Ada")).toBe("Hello, Ada");
});

test("falls back when there is no name", () => {
  expect(greeting("   ")).toBe("Hello");
  expect(greeting(undefined)).toBe("Hello");
});
