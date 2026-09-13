/* node 环境没有 localStorage；storeGet/storeSet 只在交互路径用到，给个哑实现。 */
const store = new Map<string, string>()
const g = globalThis as Record<string, unknown>
g.localStorage = {
  getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
  setItem: (k: string, v: string) => void store.set(k, String(v)),
  removeItem: (k: string) => void store.delete(k),
  clear: () => void store.clear(),
  key: (i: number) => [...store.keys()][i] ?? null,
  get length() { return store.size },
}
