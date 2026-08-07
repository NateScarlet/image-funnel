import { mdiStar, mdiStarOutline } from "@mdi/js";

export interface StarConfig {
  value: number;
  colorClass: string;
  colorVar: string;
  label: string;
  filledIcon: string;
  outlineIcon: string;
}

export const STAR_CONFIGS: StarConfig[] = [
  {
    value: 0,
    colorClass: "text-primary-500",
    colorVar: "var(--color-primary-500)",
    label: "未评分",
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline,
  },
  {
    value: 1,
    colorClass: "text-red-500",
    colorVar: "var(--color-red-500)",
    label: "1星",
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline,
  },
  {
    value: 2,
    colorClass: "text-orange-500",
    colorVar: "var(--color-orange-500)",
    label: "2星",
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline,
  },
  {
    value: 3,
    colorClass: "text-green-500",
    colorVar: "var(--color-green-500)",
    label: "3星",
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline,
  },
  {
    value: 4,
    colorClass: "text-blue-500",
    colorVar: "var(--color-blue-500)",
    label: "4星",
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline,
  },
  {
    value: 5,
    colorClass: "text-purple-500",
    colorVar: "var(--color-purple-500)",
    label: "5星",
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline,
  },
];

function getStarConfig(rating: number): StarConfig {
  return STAR_CONFIGS.find((s) => s.value === rating) ?? STAR_CONFIGS[0];
}

export function getStarColorClass(rating: number): string {
  return getStarConfig(rating).colorClass;
}

export function getStarColorVar(rating: number): string {
  return getStarConfig(rating).colorVar;
}
