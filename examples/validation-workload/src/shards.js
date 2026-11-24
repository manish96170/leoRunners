export function shardOf(index, total) {
  if (
    !Number.isInteger(index) ||
    !Number.isInteger(total) ||
    total < 1 ||
    index < 1 ||
    index > total
  ) {
    throw new RangeError("shard index must be within 1..total");
  }
  return `${index}/${total}`;
}
