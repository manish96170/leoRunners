import { expect, test } from "vitest";
import { count } from "./counter.js";

test("counts matching items", () => {
  expect(count([1, 2, 3, 4], (n) => n % 2 === 0)).toBe(2);
});

test("counts nothing in an empty list", () => {
  expect(count([], () => true)).toBe(0);
});
