/**
 * Plain text of provider HTML. DOMParser builds an inert document, so no
 * script runs and no resource loads; only the text content is kept.
 */
export function htmlToText(value: string): string {
  if (!value.includes("<")) return value;
  if (typeof DOMParser === "undefined") return value.replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim();
  const document = new DOMParser().parseFromString(value, "text/html");
  document.querySelectorAll("br").forEach((element) => element.replaceWith("\n"));
  document.querySelectorAll("p, div, li").forEach((element) => element.append("\n"));
  return (document.body.textContent ?? "").replace(/\n{3,}/g, "\n\n").trim();
}
