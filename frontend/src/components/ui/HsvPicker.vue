<template>
  <div class="space-y-2 select-none">
    <!-- 饱和度与明度面板 -->
    <div ref="svWrapper" class="relative w-full rounded-lg overflow-hidden" style="height:110px">
      <canvas ref="svCanvas"
              class="absolute inset-0 w-full h-full block"
              style="cursor: crosshair;"
              @mousedown="svDown"
              @touchstart.prevent="svTouchStart" @touchmove.prevent="svTouchMove" @touchend="onGlobalMouseUp" />
    </div>
    <!-- 色相滑条 -->
    <div ref="hueWrapper" class="relative w-full rounded-full overflow-hidden" style="height:14px">
      <canvas ref="hueCanvas"
              class="absolute inset-0 w-full h-full block"
              style="cursor: pointer;"
              @mousedown="hueDown"
              @touchstart.prevent="hueTouchStart" @touchmove.prevent="hueTouchMove" @touchend="onGlobalMouseUp" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'

interface RGBColor { r: number; g: number; b: number }
const props = defineProps<{ color: RGBColor }>()
const emit = defineEmits<{ change: [c: RGBColor] }>()

const svCanvas   = ref<HTMLCanvasElement | null>(null)
const hueCanvas  = ref<HTMLCanvasElement | null>(null)
const svWrapper  = ref<HTMLElement | null>(null)
const hueWrapper = ref<HTMLElement | null>(null)

const draggingSV  = ref(false)
const draggingHue = ref(false)
const prevColor   = ref<RGBColor>(props.color)
const hsv         = ref<[number, number, number]>(rgbToHsv(props.color.r, props.color.g, props.color.b))

// 同步画布尺寸
let roSV:  ResizeObserver | null = null
let roHue: ResizeObserver | null = null

onMounted(() => {
  // 同步饱和度与明度画布尺寸
  roSV = new ResizeObserver(() => {
    const el = svCanvas.value; const w = svWrapper.value
    if (!el || !w) return
    el.width  = w.clientWidth
    el.height = w.clientHeight
    drawSV()
  })
  if (svWrapper.value) roSV.observe(svWrapper.value)

  // 同步色相画布尺寸
  roHue = new ResizeObserver(() => {
    const el = hueCanvas.value; const w = hueWrapper.value
    if (!el || !w) return
    el.width  = w.clientWidth
    el.height = w.clientHeight
    drawHue()
  })
  if (hueWrapper.value) roHue.observe(hueWrapper.value)

  draw()
})

onUnmounted(() => { roSV?.disconnect(); roHue?.disconnect() })

watch(() => props.color, (c) => {
  const p = prevColor.value
  if (c.r !== p.r || c.g !== p.g || c.b !== p.b) {
    prevColor.value = c
    hsv.value = rgbToHsv(c.r, c.g, c.b)
    draw()
  }
})
watch(hsv, () => draw(), { deep: true })

function draw() { drawSV(); drawHue() }

function drawSV() {
  const canvas = svCanvas.value; if (!canvas) return
  const ctx = canvas.getContext('2d'); if (!ctx) return
  const w = canvas.width, h = canvas.height
  if (!w || !h) return

  // 绘制底色
  ctx.fillStyle = `hsl(${hsv.value[0]},100%,50%)`
  ctx.fillRect(0, 0, w, h)
  // 绘制白色横向渐变层
  const sg = ctx.createLinearGradient(0, 0, w, 0)
  sg.addColorStop(0, 'rgba(255,255,255,1)'); sg.addColorStop(1, 'rgba(255,255,255,0)')
  ctx.fillStyle = sg; ctx.fillRect(0, 0, w, h)
  // 绘制黑色纵向渐变层
  const vg = ctx.createLinearGradient(0, 0, 0, h)
  vg.addColorStop(0, 'rgba(0,0,0,0)'); vg.addColorStop(1, 'rgba(0,0,0,1)')
  ctx.fillStyle = vg; ctx.fillRect(0, 0, w, h)

  // 绘制圆形游标
  const cx = Math.round(hsv.value[1] * w)
  const cy = Math.round((1 - hsv.value[2]) * h)
  const R = 6
  ctx.beginPath(); ctx.arc(cx, cy, R + 1, 0, Math.PI * 2)
  ctx.strokeStyle = 'rgba(0,0,0,0.4)'; ctx.lineWidth = 1.5; ctx.stroke()
  ctx.beginPath(); ctx.arc(cx, cy, R, 0, Math.PI * 2)
  ctx.strokeStyle = 'rgba(255,255,255,0.95)'; ctx.lineWidth = 2; ctx.stroke()
}

function drawHue() {
  const canvas = hueCanvas.value; if (!canvas) return
  const ctx = canvas.getContext('2d'); if (!ctx) return
  const w = canvas.width, h = canvas.height
  if (!w || !h) return

  // 绘制色相渐变条
  const g = ctx.createLinearGradient(0, 0, w, 0)
  const stops: [number, string][] = [
    [0,'#ff0000'],[1/6,'#ffaa00'],[2/6,'#ffff00'],
    [3/6,'#00ff00'],[4/6,'#00aaff'],[5/6,'#7700ff'],[1,'#ff0000']
  ]
  stops.forEach(([p, c]) => g.addColorStop(p, c))
  ctx.fillStyle = g; ctx.fillRect(0, 0, w, h)

  // 绘制拖动指示条
  const cx = Math.round((hsv.value[0] / 360) * w)
  const barW = 4, barH = h - 2, rx = 2
  const bx = Math.max(barW / 2, Math.min(w - barW / 2, cx)) - barW / 2
  ctx.beginPath()
  if (ctx.roundRect) {
    ctx.roundRect(bx, 1, barW, barH, rx)
  } else {
    ctx.rect(bx, 1, barW, barH)
  }
  ctx.fillStyle = 'rgba(255,255,255,0.95)'; ctx.fill()
  ctx.strokeStyle = 'rgba(0,0,0,0.3)'; ctx.lineWidth = 1; ctx.stroke()
}

// 交互处理
function pickSV(e: { clientX: number; clientY: number }) {
  const canvas = svCanvas.value; if (!canvas) return
  const r = canvas.getBoundingClientRect()
  const s = Math.max(0, Math.min(1, (e.clientX - r.left) / r.width))
  const v = Math.max(0, Math.min(1, 1 - (e.clientY - r.top) / r.height))
  hsv.value = [hsv.value[0], s, v]
  const rgb = hsvToRgb(hsv.value[0], s, v)
  prevColor.value = rgb; emit('change', rgb)
}

function pickHue(e: { clientX: number }) {
  const canvas = hueCanvas.value; if (!canvas) return
  const r = canvas.getBoundingClientRect()
  const h = Math.max(0, Math.min(360, ((e.clientX - r.left) / r.width) * 360))
  hsv.value = [h, hsv.value[1], hsv.value[2]]
  const rgb = hsvToRgb(h, hsv.value[1], hsv.value[2])
  prevColor.value = rgb; emit('change', rgb)
}

const svDown = (e: MouseEvent) => { draggingSV.value = true; pickSV(e) }
const svTouchStart = (e: TouchEvent) => { draggingSV.value = true; pickSV(e.touches[0]) }
const svTouchMove  = (e: TouchEvent) => { if (draggingSV.value) pickSV(e.touches[0]) }

const hueDown = (e: MouseEvent) => { draggingHue.value = true; pickHue(e) }
const hueTouchStart = (e: TouchEvent) => { draggingHue.value = true; pickHue(e.touches[0]) }
const hueTouchMove  = (e: TouchEvent) => { if (draggingHue.value) pickHue(e.touches[0]) }

// 监听全局拖拽
function onGlobalMouseMove(e: MouseEvent) {
  if (draggingSV.value) pickSV(e)
  else if (draggingHue.value) pickHue(e)
}
function onGlobalMouseUp() {
  draggingSV.value = false
  draggingHue.value = false
}
onMounted(() => {
  document.addEventListener('mousemove', onGlobalMouseMove)
  document.addEventListener('mouseup', onGlobalMouseUp)
})
onUnmounted(() => {
  document.removeEventListener('mousemove', onGlobalMouseMove)
  document.removeEventListener('mouseup', onGlobalMouseUp)
})

// 颜色空间转换
function rgbToHsv(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b), d = max - min
  let h = 0; const s = max === 0 ? 0 : d / max; const v = max
  if (d !== 0) {
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break
      case g: h = ((b - r) / d + 2) / 6; break
      case b: h = ((r - g) / d + 4) / 6; break
    }
  }
  return [h * 360, s, v]
}

function hsvToRgb(h: number, s: number, v: number): RGBColor {
  h /= 360; const i = Math.floor(h * 6), f = h * 6 - i
  const p = v * (1 - s), q = v * (1 - f * s), t = v * (1 - (1 - f) * s)
  let r = 0, g = 0, b = 0
  switch (i % 6) {
    case 0: r=v; g=t; b=p; break; case 1: r=q; g=v; b=p; break
    case 2: r=p; g=v; b=t; break; case 3: r=p; g=q; b=v; break
    case 4: r=t; g=p; b=v; break; case 5: r=v; g=p; b=q; break
  }
  return { r: Math.round(r * 255), g: Math.round(g * 255), b: Math.round(b * 255) }
}
</script>
