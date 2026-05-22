export default async function openURL(
  url: string,
  {
    target = "_self",
    timeout = 3e3,
  }: {
    target?: string;
    timeout?: number;
  } = {},
): Promise<boolean> {
  if (navigator.userActivation?.isActive === false) {
    return false;
  }
  return new Promise((resolve) => {
    window.focus();
    window.addEventListener(
      "blur",
      () => {
        resolve(true);
      },
      { once: true },
    );
    window.open(url, target);
    setTimeout(async () => {
      resolve(false);
    }, timeout);
  });
}
