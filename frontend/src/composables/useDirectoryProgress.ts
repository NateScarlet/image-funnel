const STORAGE_KEY_PREFIX = "directoryOrder@122b3b541739-";

function getStorageKey(parentId: string): string {
  return `${STORAGE_KEY_PREFIX}${parentId}`;
}

function recordDirectoryOrder(parentId: string, directoryIds: string[]): void {
  const key = getStorageKey(parentId);
  localStorage.setItem(key, JSON.stringify(directoryIds));
}

function getNextDirectory(
  parentId: string,
  currentDirectoryId: string,
): string | undefined {
  const key = getStorageKey(parentId);
  const rawValue = localStorage.getItem(key);
  if (!rawValue) {
    return undefined;
  }

  try {
    const directoryIds = JSON.parse(rawValue) as string[];
    const currentIndex = directoryIds.findIndex(
      (id) => id === currentDirectoryId,
    );
    if (currentIndex === -1 || currentIndex === directoryIds.length - 1) {
      return undefined;
    }
    return directoryIds[currentIndex + 1];
  } catch {
    return undefined;
  }
}

export default function useDirectoryProgress() {
  return {
    recordDirectoryOrder,
    getNextDirectory,
  };
}
