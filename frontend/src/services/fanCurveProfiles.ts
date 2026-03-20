import { main, types } from '../../wailsjs/go/models'
import { apiService } from './api'

export const DEVICE_PROFILE_ID = 'device-default'

export interface FanCurveProfile {
  id: string
  name: string
  curve: types.FanCurvePoint[]
}

const profileIdFromName = (name: string, index: number) =>
  `profile-${name.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9\-\u4e00-\u9fa5]/g, '') || 'cfg'}-${index + 1}`

const cloneCurve = (curve: types.FanCurvePoint[]) =>
  (curve || []).map(p =>
    types.FanCurvePoint.createFrom({
      temperature: p.temperature,
      rpm: p.rpm,
      offset: p.offset || 0,
    })
  )

export async function loadCustomProfiles(): Promise<FanCurveProfile[]> {
  try {
    const profiles = await apiService.getFanCurveProfileConfigs()
    return (profiles || [])
      .filter(item => item && item.name && Array.isArray(item.fanCurve))
      .map((item, index) => ({
        id: profileIdFromName(String(item.name).trim() || '未命名配置', index),
        name: String(item.name).trim() || '未命名配置',
        curve: cloneCurve(item.fanCurve || []),
      }))
  } catch {
    return []
  }
}

export async function saveCustomProfiles(profiles: FanCurveProfile[]) {
  const payload: main.FanCurveProfileConfig[] = profiles
    .filter(profile => profile.id !== DEVICE_PROFILE_ID)
    .map(profile => ({
      // 仅按名称持久化，由后端落盘为 <name>-fan-config.json
      name: (profile.name || '').trim() || '未命名配置',
      fanCurve: cloneCurve(profile.curve),
    }))
    .map(item => main.FanCurveProfileConfig.createFrom(item as any))

  await apiService.saveFanCurveProfileConfigs(payload)
}

