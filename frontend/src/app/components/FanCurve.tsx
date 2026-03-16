import React, { useState, useEffect, useCallback, memo, useMemo, useRef } from 'react'
import { Check, ChevronLeft, ChevronRight, MoreHorizontal, RefreshCw, SlidersHorizontal } from 'lucide-react'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import { apiService } from '../services/api'
import { logger } from '../services/logger'
import { types } from '../../../wailsjs/go/models'
import { ToggleSwitch, Select, Button, Badge, Card } from './ui'
import clsx from 'clsx'

interface FanCurveProps {
  config: types.AppConfig
  onConfigChange: (config: types.AppConfig) => void
  isConnected: boolean
  fanData: types.FanData | null
  temperature: types.TemperatureData | null
}

// 温度指示线独立组件，不触发图表重绘
const TemperatureIndicator = memo(function TemperatureIndicator({
  temperature, chartRef, temperatureRange,
}: {
  temperature: number | null
  chartRef: React.RefObject<HTMLDivElement | null>
  temperatureRange: { min: number; max: number }
}) {
  const [pos, setPos] = useState<{ x: number; top: number; height: number } | null>(null)

  useEffect(() => {
    if (temperature === null || !chartRef.current) { setPos(null); return }
    const update = () => {
      const grid = chartRef.current?.querySelector('.recharts-cartesian-grid')
      const container = chartRef.current?.querySelector('.recharts-responsive-container')
      if (!grid || !container) return
      const gr = grid.getBoundingClientRect()
      const cr = container.getBoundingClientRect()
      const pct = (temperature - temperatureRange.min) / (temperatureRange.max - temperatureRange.min)
      setPos({ x: (gr.left - cr.left) + pct * gr.width, top: gr.top - cr.top, height: gr.height })
    }
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [temperature, chartRef, temperatureRange])

  if (!pos || temperature === null) return null
  return (
    <svg className="absolute inset-0 pointer-events-none overflow-visible" style={{ width: '100%', height: '100%' }}>
      <line x1={pos.x} y1={pos.top} x2={pos.x} y2={pos.top + pos.height}
        stroke="#ef4444" strokeWidth={2} strokeDasharray="5 5" />
      <rect x={pos.x - 45} y={pos.top - 22} width={90} height={20} rx={4} fill="#ef4444" />
      <text x={pos.x} y={pos.top - 8} textAnchor="middle" fill="white" fontSize={11} fontWeight={500}>
        当前 {temperature}°C
      </text>
    </svg>
  )
})

// 可拖拽圆点
const DraggablePoint = memo(function DraggablePoint({
  cx, cy, index, rpm, onDragStart, isActive,
}: {
  cx: number; cy: number; index: number; temperature: number; rpm: number
  onDragStart: (index: number) => void; isActive: boolean
}) {
  const down = useCallback((e: React.MouseEvent) => { e.preventDefault(); e.stopPropagation(); onDragStart(index) }, [index, onDragStart])
  const touch = useCallback((e: React.TouchEvent) => { e.preventDefault(); e.stopPropagation(); onDragStart(index) }, [index, onDragStart])

  return (
    <g>
      <circle cx={cx} cy={cy} r={isActive ? 14 : 10} fill="transparent" stroke="transparent"
        style={{ cursor: 'ns-resize' }} onMouseDown={down} onTouchStart={touch} />
      <circle cx={cx} cy={cy} r={isActive ? 8 : 6}
        fill={isActive ? '#1d4ed8' : '#3b82f6'} stroke="white" strokeWidth={2}
        style={{
          cursor: 'ns-resize',
          transition: isActive ? 'none' : 'all 0.2s ease',
          filter: isActive
            ? 'drop-shadow(0 4px 8px rgba(59,130,246,0.5))'
            : 'drop-shadow(0 2px 4px rgba(0,0,0,0.1))',
        }}
        onMouseDown={down} onTouchStart={touch} />
      {isActive && (
        <g>
          <rect x={cx - 35} y={cy - 35} width={70} height={24} rx={4} fill="#1e40af" opacity={0.95} />
          <text x={cx} y={cy - 19} textAnchor="middle" fill="white" fontSize={12} fontWeight={600}>
            {rpm} RPM
          </text>
        </g>
      )}
    </g>
  )
})

// 主组件
const FanCurve = memo(function FanCurve({ config, onConfigChange, isConnected, fanData, temperature }: FanCurveProps) {
  const [localCurve, setLocalCurve] = useState<types.FanCurvePoint[]>([])
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [isInitialized, setIsInitialized] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isDarkMode, setIsDarkMode] = useState(false)

  // 拖拽 & 选中
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null)
  const [isInteracting, setIsInteracting] = useState(false)

  const chartRef = useRef<HTMLDivElement>(null)
  const chartBoundsRef = useRef<{ top: number; bottom: number; yMin: number; yMax: number } | null>(null)

  const [rpmRange, setRpmRange] = useState({ min: 500, max: 4000, ticks: [500, 1000, 1500, 2000, 2500, 3000, 3500, 4000] })

  const temperatureRange = useMemo(() => ({
    min: 30, max: 100,
    ticks: Array.from({ length: 15 }, (_, i) => 30 + i * 5),
  }), [])

  // 深色模式监听
  useEffect(() => {
    const root = document.documentElement
    const update = () => setIsDarkMode(root.classList.contains('dark'))
    update()
    const obs = new MutationObserver(update)
    obs.observe(root, { attributes: true, attributeFilter: ['class'] })
    return () => obs.disconnect()
  }, [])

  // 初始化曲线
  useEffect(() => {
    if (isInitialized || !config.fanCurve?.length) return
    const curve = [...config.fanCurve]
    const last = curve[curve.length - 1]
    if (last.temperature < 100) curve.push({ temperature: 100, rpm: last.rpm, offset: 0 })
    setLocalCurve(curve)
    setIsInitialized(true)
    if (fanData?.maxGear) {
      const maxRpm = fanData.maxGear === '标准' ? 2760 : fanData.maxGear === '强劲' ? 3300 : 4000
      const ticks = Array.from({ length: Math.floor((maxRpm - 500) / 500) + 1 }, (_, i) => 500 + i * 500)
      setRpmRange({ min: 500, max: maxRpm, ticks })
    }
  }, [config.fanCurve, fanData?.maxGear, isInitialized])

  // 同步后端偏移
  useEffect(() => {
    if (!isInitialized || !config.fanCurveOffsetEnabled) return
    setLocalCurve(prev => {
      let changed = false
      const next = prev.map((p, i) => {
        const cfgOffset = config.fanCurve[i]?.offset ?? 0
        if (p.offset === cfgOffset) return p
        changed = true
        return { ...p, offset: cfgOffset }
      })
      return changed ? next : prev
    })
  }, [config.fanCurve, config.fanCurveOffsetEnabled, isInitialized])

  // 图表数据
  const chartData = useMemo(() => localCurve.map((p, index) => {
    const offset = config.fanCurveOffsetEnabled ? (p.offset || 0) : 0
    const effectiveRpm = Math.min(rpmRange.max, Math.max(rpmRange.min, Math.round((p.rpm + offset) / 100) * 100))
    return {
      temperature: p.temperature,
      rpm: p.rpm,
      effectiveRpm: config.fanCurveOffsetEnabled ? effectiveRpm : undefined,
      index,
    }
  }), [localCurve, config.fanCurveOffsetEnabled, rpmRange])

  const updatePoint = useCallback((index: number, newRpm: number) => {
    const clamped = Math.max(rpmRange.min, Math.min(rpmRange.max, Math.round(newRpm / 100) * 100))
    setLocalCurve(prev => {
      if (prev[index]?.rpm === clamped) return prev
      const next = [...prev]
      next[index] = { ...next[index], rpm: clamped }
      return next
    })
    setHasUnsavedChanges(true)
  }, [rpmRange])

  // 拖拽
  const handleDragStart = useCallback((index: number) => {
    setDragIndex(index)
    setSelectedIndex(index)
    setIsInteracting(true)
    if (chartRef.current) {
      const grid = chartRef.current.querySelector('.recharts-cartesian-grid')
      if (grid) {
        const r = grid.getBoundingClientRect()
        chartBoundsRef.current = { top: r.top, bottom: r.bottom, yMin: rpmRange.min, yMax: rpmRange.max }
      }
    }
  }, [rpmRange])

  const handleDrag = useCallback((clientY: number) => {
    if (dragIndex === null || !chartBoundsRef.current) return
    const { top, bottom, yMin, yMax } = chartBoundsRef.current
    const rel = Math.max(0, Math.min(1, (bottom - clientY) / (bottom - top)))
    updatePoint(dragIndex, yMin + rel * (yMax - yMin))
  }, [dragIndex, updatePoint])

  const handleDragEnd = useCallback(() => {
    setDragIndex(null)
    setTimeout(() => setIsInteracting(false), 100)
  }, [])

  useEffect(() => {
    if (dragIndex === null) return
    const onMove = (e: MouseEvent) => { e.preventDefault(); handleDrag(e.clientY) }
    const onTouch = (e: TouchEvent) => { if (e.touches.length) handleDrag(e.touches[0].clientY) }
    const onEnd = () => handleDragEnd()
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onEnd)
    document.addEventListener('touchmove', onTouch, { passive: false })
    document.addEventListener('touchend', onEnd)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onEnd)
      document.removeEventListener('touchmove', onTouch)
      document.removeEventListener('touchend', onEnd)
    }
  }, [dragIndex, handleDrag, handleDragEnd])

  // 保存
  const saveCurve = useCallback(async () => {
    if (isSaving) return
    try {
      setIsSaving(true)
      await apiService.setFanCurve(localCurve)
      onConfigChange(types.AppConfig.createFrom({ ...config, fanCurve: localCurve }))
      setHasUnsavedChanges(false)
    } catch (e) { logger.error('保存风扇曲线失败', 'FanCurve', e) }
    finally { setIsSaving(false) }
  }, [localCurve, config, onConfigChange, isSaving])

  // 重置
  const resetCurve = useCallback(() => {
    const m = rpmRange.max
    setLocalCurve([
      { temperature: 30, rpm: 500, offset: 0 },
      { temperature: 35, rpm: 1200, offset: 0 },
      { temperature: 40, rpm: 1400, offset: 0 },
      { temperature: 45, rpm: 1600, offset: 0 },
      { temperature: 50, rpm: 1800, offset: 100 },
      { temperature: 55, rpm: 2000, offset: 100 },
      { temperature: 60, rpm: Math.min(2300, m), offset: 100 },
      { temperature: 65, rpm: Math.min(2600, m), offset: 200 },
      { temperature: 70, rpm: Math.min(2900, m), offset: 200 },
      { temperature: 75, rpm: Math.min(3200, m), offset: 200 },
      { temperature: 80, rpm: Math.min(3500, m), offset: 300 },
      { temperature: 85, rpm: Math.min(3800, m), offset: 200 },
      { temperature: 90, rpm: m, offset: 0 },
      { temperature: 95, rpm: m, offset: 0 },
      { temperature: 100, rpm: m, offset: 0 },
    ])
    setHasUnsavedChanges(true)
  }, [rpmRange.max])

  // 导出
  const exportFanConfig = useCallback(() => {
    const blob = new Blob([JSON.stringify({
      fanCurve: localCurve, autoControl: config.autoControl,
      manualGear: config.manualGear, manualLevel: config.manualLevel,
      exportDate: new Date().toISOString(), version: '1.0',
    }, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = Object.assign(document.createElement('a'), {
      href: url, download: `bs2pro-fan-config-${new Date().toISOString().split('T')[0]}.json`,
    })
    document.body.appendChild(a); a.click(); document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }, [localCurve, config])

  // 导入
  const importFanConfig = useCallback(() => {
    const input = Object.assign(document.createElement('input'), { type: 'file', accept: '.json' })
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0]
      if (!file) return
      const reader = new FileReader()
      reader.onload = async (ev) => {
        try {
          const fc = JSON.parse(ev.target?.result as string)
          if (!Array.isArray(fc.fanCurve)) throw new Error('无效的风扇配置文件')
          setLocalCurve(fc.fanCurve)
          setHasUnsavedChanges(true)
          if (fc.autoControl !== undefined) {
            await apiService.setAutoControl(fc.autoControl).catch(() => {})
            onConfigChange(types.AppConfig.createFrom({ ...config, autoControl: fc.autoControl }))
          }
        } catch (err) { logger.error('导入风扇配置失败', 'FanCurve', err); alert('导入失败：文件格式无效') }
      }
      reader.readAsText(file)
    }
    input.click()
  }, [config, onConfigChange])

  const handleOffsetToggle = useCallback(async (enabled: boolean) => {
    try {
      const nc = types.AppConfig.createFrom({ ...config, fanCurveOffsetEnabled: enabled })
      await apiService.updateConfig(nc); onConfigChange(nc)
    } catch (e) { logger.error('设置风扇偏移失败', 'FanCurve', e) }
  }, [config, onConfigChange])

  const handleGearChange = useCallback(async (gear: string) => {
    try {
      await apiService.setManualGear(gear, config.manualLevel || '中')
      onConfigChange(types.AppConfig.createFrom({ ...config, manualGear: gear }))
    } catch (e) { logger.error('设置手动挡位失败', 'FanCurve', e) }
  }, [config, onConfigChange])

  const handleLevelChange = useCallback(async (level: string) => {
    try {
      await apiService.setManualGear(config.manualGear || '标准', level)
      onConfigChange(types.AppConfig.createFrom({ ...config, manualLevel: level }))
    } catch (e) { logger.error('设置挡位级别失败', 'FanCurve', e) }
  }, [config, onConfigChange])

  const CustomDot = useCallback((props: any): React.ReactElement<SVGElement> => {
    const { cx, cy, index, payload } = props
    if (cx === undefined || cy === undefined) return <g />
    return (
      <DraggablePoint
        key={`dot-${index}`}
        cx={cx} cy={cy} index={index}
        temperature={payload.temperature} rpm={payload.rpm}
        onDragStart={handleDragStart}
        isActive={dragIndex === index}
      />
    )
  }, [dragIndex, handleDragStart])

  // 当前选中点数据
  const selectedPoint = selectedIndex !== null ? localCurve[selectedIndex] : null
  const selectedOffset = selectedPoint && config.fanCurveOffsetEnabled ? (selectedPoint.offset || 0) : 0
  const selectedEffective = selectedPoint
    ? Math.min(rpmRange.max, Math.max(rpmRange.min, Math.round((selectedPoint.rpm + selectedOffset) / 100) * 100))
    : 0

  const gearOptions = [
    { value: '静音', label: '静音' }, { value: '标准', label: '标准' },
    { value: '强劲', label: '强劲' }, { value: '超频', label: '超频' },
  ]
  const levelOptions = [{ value: '低', label: '低' }, { value: '中', label: '中' }, { value: '高', label: '高' }]

  return (
    <Card className="p-3">

      {/* 头部 */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-1.5">
          {hasUnsavedChanges && <Badge variant="warning" size="sm">未保存</Badge>}
          {isInteracting && <Badge variant="info" size="sm">编辑中</Badge>}
        </div>
        {!config.autoControl && isConnected && (
          <div className="flex items-center gap-1.5">
            <span className="text-xs text-gray-500 dark:text-gray-400">手动挡位</span>
            <Select value={config.manualGear || '标准'} onChange={handleGearChange} options={gearOptions} size="sm" />
            <Select value={config.manualLevel || '中'} onChange={handleLevelChange} options={levelOptions} size="sm" />
          </div>
        )}
      </div>

      {/* 图表 */}
      <div
        ref={chartRef}
        className={clsx(
          'relative rounded-xl border bg-white dark:bg-gray-800 p-2 mb-3',
          'border-gray-200 dark:border-gray-700',
          dragIndex !== null && 'ring-2 ring-blue-500 ring-opacity-50',
        )}
      >
        <div className="h-68 md:h-76 relative">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData} margin={{ top: 10, right: 20, left: 10, bottom: 15 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" className="dark:stroke-gray-600" />
              <XAxis
                dataKey="temperature" type="number"
                domain={[temperatureRange.min, temperatureRange.max]}
                ticks={temperatureRange.ticks} tickLine={false}
                axisLine={{ stroke: '#d1d5db' }} tick={{ fill: '#6b7280', fontSize: 11 }}
                allowDataOverflow
                label={{ value: '温度 (°C)', position: 'insideBottom', offset: -10, fill: '#6b7280', fontSize: 12 }}
              />
              <YAxis
                type="number" domain={[rpmRange.min, rpmRange.max]} ticks={rpmRange.ticks}
                tickLine={false} axisLine={{ stroke: '#d1d5db' }} tick={{ fill: '#6b7280', fontSize: 11 }}
                allowDataOverflow
                label={{ value: '转速 (RPM)', angle: -90, position: 'insideLeft', fill: '#6b7280', fontSize: 12 }}
              />
              <Tooltip
                formatter={(value: any, name: any) => {
                  const v = typeof value === 'number' ? value : 0
                  return name === 'effectiveRpm' ? [`${v} RPM`, '实际转速(含偏移)'] : [`${v} RPM`, '基础转速']
                }}
                labelFormatter={v => `温度: ${v}°C`}
                contentStyle={{
                  backgroundColor: isDarkMode ? 'rgba(17,24,39,0.95)' : 'rgba(255,255,255,0.95)',
                  border: '1px solid', borderColor: isDarkMode ? '#374151' : 'transparent',
                  borderRadius: '8px',
                  boxShadow: isDarkMode ? '0 10px 25px -5px rgba(0,0,0,0.6)' : '0 10px 25px -5px rgba(0,0,0,0.1)',
                  padding: '8px 12px', color: isDarkMode ? '#e5e7eb' : '#111827',
                }}
                labelStyle={{ color: isDarkMode ? '#e5e7eb' : '#111827', fontWeight: 600 }}
                itemStyle={{ color: isDarkMode ? '#e5e7eb' : '#111827' }}
              />
              <Line type="monotone" dataKey="rpm" stroke="#3b82f6" strokeWidth={3}
                dot={CustomDot} activeDot={false} isAnimationActive={false} name="rpm" />
              {config.fanCurveOffsetEnabled && (
                <Line type="monotone" dataKey="effectiveRpm" stroke="#f59e0b" strokeWidth={2}
                  strokeDasharray="6 3" dot={false} activeDot={false} isAnimationActive={false} name="effectiveRpm" />
              )}
            </LineChart>
          </ResponsiveContainer>
          <TemperatureIndicator
            temperature={temperature?.maxTemp ?? null}
            chartRef={chartRef}
            temperatureRange={temperatureRange}
          />
        </div>
      </div>

      {/* 操作栏 */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        <div className="flex items-center gap-2">
          <Button variant="primary" size="sm" onClick={saveCurve}
            disabled={!hasUnsavedChanges} loading={isSaving} icon={<Check className="w-3 h-3" />}>
            保存
          </Button>

          <Button variant="secondary" size="sm" onClick={() => {
            setLocalCurve([...config.fanCurve])
            setHasUnsavedChanges(false)
          }}
            disabled={!hasUnsavedChanges}
            icon={<RefreshCw className="w-3 h-3" />}>
            取消
          </Button>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" onClick={resetCurve}
            icon={<RefreshCw className="w-3 h-3" />}>
            重置
          </Button>

          <Button variant="secondary" size="sm" onClick={exportFanConfig}>
            导出
          </Button>

          <Button variant="secondary" size="sm" onClick={importFanConfig}>
            导入
          </Button>
        </div>

        <span className="ml-auto text-xs text-gray-400 dark:text-gray-500">
          拖拽圆点调整转速
        </span>
      </div>

      {/* 自动偏移开关 */}
      <div className="flex items-center justify-between mb-2 px-1">
        <div className="flex items-center gap-1.5">
          <SlidersHorizontal className="w-3.5 h-3.5 text-amber-500" />
          <span className="text-xs font-medium text-gray-700 dark:text-gray-300">自动曲线偏移</span>
          {config.fanCurveOffsetEnabled && (
            <Badge variant="warning" size="sm">
              {(temperature?.autoOffset ?? 0) >= 0 ? '+' : ''}{temperature?.autoOffset ?? 0} RPM
            </Badge>
          )}
        </div>
        <ToggleSwitch enabled={config.fanCurveOffsetEnabled ?? false} onChange={handleOffsetToggle} />
      </div>
      {config.fanCurveOffsetEnabled && (
        <div className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400 mb-3 px-1">
          <span className="inline-block w-4 border-t-2 border-dashed border-amber-500 flex-shrink-0" />
          <span>橙色虚线为含偏移的实际转速，系统按区间独立调节并自动收敛</span>
        </div>
      )}

      {/* 单点编辑面板 */}
      {selectedPoint && (
        <div className="rounded-xl border border-blue-200 dark:border-blue-800 bg-blue-50 dark:bg-blue-900/20 p-3">
          <div className="flex items-center justify-between mb-2">
            <div className="flex items-center gap-2">
              {/* 上一点 */}
              <button
                onClick={() => setSelectedIndex(i => i !== null && i > 0 ? i - 1 : i)}
                disabled={selectedIndex === 0}
                className="w-6 h-6 flex items-center justify-center rounded-md text-gray-500 hover:bg-blue-100 dark:hover:bg-blue-800 disabled:opacity-30 transition-colors"
              >
                <ChevronLeft className="w-4 h-4" />
              </button>
              <span className="text-sm font-semibold text-blue-700 dark:text-blue-300 min-w-[48px] text-center">
                {selectedPoint.temperature}°C
              </span>
              {/* 下一点 */}
              <button
                onClick={() => setSelectedIndex(i => i !== null && i < localCurve.length - 1 ? i + 1 : i)}
                disabled={selectedIndex === localCurve.length - 1}
                className="w-6 h-6 flex items-center justify-center rounded-md text-gray-500 hover:bg-blue-100 dark:hover:bg-blue-800 disabled:opacity-30 transition-colors"
              >
                <ChevronRight className="w-4 h-4" />
              </button>
            </div>

            <div className="flex items-center gap-2">
              {/* RPM 数字输入 */}
              <input
                type="number"
                value={selectedPoint.rpm}
                onChange={e => selectedIndex !== null && updatePoint(selectedIndex, Number(e.target.value))}
                min={rpmRange.min} max={rpmRange.max} step={100}
                className="w-24 px-2 py-1 text-sm text-center font-semibold rounded-lg border border-blue-300 dark:border-blue-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
              <span className="text-xs text-gray-500 dark:text-gray-400">RPM</span>
            </div>

            <button
              onClick={() => setSelectedIndex(null)}
              className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 transition-colors"
            >
              关闭
            </button>
          </div>

          {/* 滑块 */}
          <input
            type="range"
            value={selectedPoint.rpm}
            onChange={e => selectedIndex !== null && updatePoint(selectedIndex, Number(e.target.value))}
            min={rpmRange.min} max={rpmRange.max} step={100}
            className="w-full h-1.5 rounded-full appearance-none cursor-pointer slider-thumb"
            style={{
              background: `linear-gradient(to right,#3b82f6 0%,#3b82f6 ${((selectedPoint.rpm - rpmRange.min) / (rpmRange.max - rpmRange.min)) * 100}%,#dbeafe ${((selectedPoint.rpm - rpmRange.min) / (rpmRange.max - rpmRange.min)) * 100}%,#dbeafe 100%)`,
            }}
          />

          {/* 偏移信息 */}
          {config.fanCurveOffsetEnabled && (
            <div className="flex items-center gap-1.5 mt-2 text-xs text-amber-600 dark:text-amber-400">
              <span>偏移量 {selectedOffset >= 0 ? '+' : ''}{selectedOffset}</span>
              <span className="text-gray-400">→</span>
              <span>实际 {selectedEffective} RPM</span>
            </div>
          )}
        </div>
      )}
    </Card>
  )
})

export default FanCurve
