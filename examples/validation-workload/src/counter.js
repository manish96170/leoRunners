export function count(items, predicate) {
  return items.filter(predicate).length;
}
