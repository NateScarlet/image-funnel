import { mdiStar, mdiStarOutline } from '@mdi/js'

export interface StarConfig {
  value: number
  color: string
  label: string
  filledIcon: string
  outlineIcon: string
}

export const STAR_CONFIGS: StarConfig[] = [
  { 
    value: 0, 
    color: '#64748b', 
    label: '未评分',
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline
  },
  { 
    value: 1, 
    color: '#ef4444', 
    label: '1星',
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline
  },
  { 
    value: 2, 
    color: '#f97316', 
    label: '2星',
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline
  },
  { 
    value: 3, 
    color: '#eab308', 
    label: '3星',
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline
  },
  { 
    value: 4, 
    color: '#22c55e', 
    label: '4星',
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline
  },
  { 
    value: 5, 
    color: '#3b82f6', 
    label: '5星',
    filledIcon: mdiStar,
    outlineIcon: mdiStarOutline
  }
]

export function getStarColor(rating: number): string {
  const config = STAR_CONFIGS.find(s => s.value === rating)
  return config?.color || '#64748b'
}
