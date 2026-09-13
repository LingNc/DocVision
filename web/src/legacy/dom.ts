/* 旧页 el()/clear() 小助手：DOM 落树的统一入口（textContent，从不拼 HTML）。 */

export function el(tag: string, cls?: string | null, text?: string | null): HTMLElement {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  if (text !== undefined && text !== null) node.textContent = text
  return node
}

export function clear(node: Element): void {
  while (node.firstChild) node.removeChild(node.firstChild)
}
