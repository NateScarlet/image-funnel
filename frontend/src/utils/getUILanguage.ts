const cachedResult = (() => {
  if (typeof navigator.languages === "object") {
    const zhIndex = navigator.languages.findIndex((i) => i.startsWith("zh"));
    const enIndex = navigator.languages.findIndex((i) => i.startsWith("en"));
    if (zhIndex >= 0 && (zhIndex < enIndex || enIndex < 0)) {
      return "zh";
    }
  }
  return "en";
})();

export default function getUILanguage(): string {
  return cachedResult;
}
