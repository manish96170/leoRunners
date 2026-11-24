import { expect, test } from "vitest";
import { shardOf } from "./shards.js";

test("formats a shard", () => {
  expect(shardOf(1, 2)).toBe("1/2");
});

test("rejects an out of range shard", () => {
  expect(() => shardOf(3, 2)).toThrow(RangeError);
});
