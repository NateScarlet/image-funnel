/**
 * 在给定的同级目录顺序中，取当前目录之后的第一个「可进入」目录。
 *
 * 自动切换场景（完成视图提交后）把「未完成」作为可进入条件传入，
 * 因此查找会跨过已完成目录继续向后，找到第一个未完成目录；不存在则返回 undefined。
 * 当前目录不在列表中时同样返回 undefined，即不跨父目录继续查找。
 */
export function getNextDirectory(
  directoryIds: readonly string[],
  currentDirectoryId: string,
  isEnterable: (directoryId: string) => boolean,
): string | undefined {
  const currentIndex = directoryIds.indexOf(currentDirectoryId);
  if (currentIndex === -1) {
    return undefined;
  }

  for (let index = currentIndex + 1; index < directoryIds.length; index += 1) {
    const directoryId = directoryIds[index];
    if (isEnterable(directoryId)) {
      return directoryId;
    }
  }
  return undefined;
}
