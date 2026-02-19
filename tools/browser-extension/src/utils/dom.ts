export function textContentIncludes(doc: Document, pattern: RegExp): boolean {
  const text = doc.body?.textContent ?? "";
  return pattern.test(text);
}

export function hasSelector(doc: Document, selector: string): boolean {
  return doc.querySelector(selector) !== null;
}

export function getMetaContent(doc: Document, name: string): string {
  const element = doc.querySelector(`meta[name="${name}"], meta[property="${name}"]`);
  return element?.getAttribute("content") ?? "";
}
