import useStorage from "./useStorage";

// #region 创建筛选会话列表的筛选开关
// 首页 DirectorySelector 与筛选完成后的自动切换必须共用同一组开关，
// 否则「界面上隐藏的目录」仍可能被自动跳转选中。
export const { model: showCompletedDirectories } = useStorage<boolean>(
  localStorage,
  "showCompletedDirectories@6309f070-f3fd-42a0-85e5-e75d9ff38d6d",
  () => false,
);

export const { model: minUnratedCount } = useStorage<number>(
  localStorage,
  "minUnratedCount@fab36720-bc2d-4876-8800-47b85f20658f",
  () => 1,
);

export const { model: showSmallUnrated } = useStorage<boolean>(
  localStorage,
  "showSmallUnrated@5918e244-67ad-4971-8608-f404494c25f4",
  () => false,
);
// #endregion
