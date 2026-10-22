// 按需导入的 MDI 图标（SVG path），避免引入 3.5MB 字体文件。
// 只导出项目实际用到的图标，tree-shaking 后体积极小。
import {
  mdiAccountGroup,
  mdiAirplane,
  mdiArrowTopRight,
  mdiChartDonut,
  mdiChartTimelineVariantShimmer,
  mdiCheck,
  mdiChevronLeft,
  mdiChevronRight,
  mdiChevronUp,
  mdiClose,
  mdiCodeBraces,
  mdiCogOutline,
  mdiContentCopy,
  mdiContentSave,
  mdiCubeOutline,
  mdiDeleteOutline,
  mdiDownloadNetwork,
  mdiEye,
  mdiEyeOff,
  mdiEyeOutline,
  mdiFileCodeOutline,
  mdiFilterOutline,
  mdiFilterRemoveOutline,
  mdiHeartPulse,
  mdiHexagonMultiple,
  mdiLinkVariant,
  mdiLinkVariantOff,
  mdiLockOutline,
  mdiLogout,
  mdiMagnify,
  mdiMagnifyPlusOutline,
  mdiOpenInNew,
  mdiPencilOutline,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiServerNetwork,
  mdiShieldHalfFull,
  mdiStar,
  mdiStarOutline,
  mdiSwapVerticalBold,
  mdiTextBoxOutline,
  mdiViewDashboardOutline,
  mdiWeatherNight,
  mdiWeatherSunny,
} from '@mdi/js'

/** 图标名（mdi-xxx）→ SVG path 的映射表 */
export const mdiIcons: Record<string, string> = {
  'mdi-account-group': mdiAccountGroup,
  'mdi-airplane': mdiAirplane,
  'mdi-arrow-top-right': mdiArrowTopRight,
  'mdi-chart-donut': mdiChartDonut,
  'mdi-chart-timeline-variant-shimmer': mdiChartTimelineVariantShimmer,
  'mdi-check': mdiCheck,
  'mdi-chevron-left': mdiChevronLeft,
  'mdi-chevron-right': mdiChevronRight,
  'mdi-chevron-up': mdiChevronUp,
  'mdi-close': mdiClose,
  'mdi-code-braces': mdiCodeBraces,
  'mdi-cog-outline': mdiCogOutline,
  'mdi-content-copy': mdiContentCopy,
  'mdi-content-save': mdiContentSave,
  'mdi-cube-outline': mdiCubeOutline,
  'mdi-delete-outline': mdiDeleteOutline,
  'mdi-download-network': mdiDownloadNetwork,
  'mdi-eye': mdiEye,
  'mdi-eye-off': mdiEyeOff,
  'mdi-eye-outline': mdiEyeOutline,
  'mdi-file-code-outline': mdiFileCodeOutline,
  'mdi-filter-outline': mdiFilterOutline,
  'mdi-filter-remove-outline': mdiFilterRemoveOutline,
  'mdi-heart-pulse': mdiHeartPulse,
  'mdi-hexagon-multiple': mdiHexagonMultiple,
  'mdi-link-variant': mdiLinkVariant,
  'mdi-link-variant-off': mdiLinkVariantOff,
  'mdi-lock-outline': mdiLockOutline,
  'mdi-logout': mdiLogout,
  'mdi-magnify': mdiMagnify,
  'mdi-magnify-plus-outline': mdiMagnifyPlusOutline,
  'mdi-open-in-new': mdiOpenInNew,
  'mdi-pencil-outline': mdiPencilOutline,
  'mdi-play': mdiPlay,
  'mdi-plus': mdiPlus,
  'mdi-refresh': mdiRefresh,
  'mdi-server-network': mdiServerNetwork,
  'mdi-shield-half-full': mdiShieldHalfFull,
  'mdi-star': mdiStar,
  'mdi-star-outline': mdiStarOutline,
  'mdi-swap-vertical-bold': mdiSwapVerticalBold,
  'mdi-text-box-outline': mdiTextBoxOutline,
  'mdi-view-dashboard-outline': mdiViewDashboardOutline,
  'mdi-weather-night': mdiWeatherNight,
  'mdi-weather-sunny': mdiWeatherSunny,
}

/**
 * 创建 Vuetify 的自定义图标集：直接返回 SVG path，
 * 组件中用 icon="mdi-xxx" 即可正常渲染。
 */
export function createMdiSvgSet() {
  return {
    component: (props: { icon: string }) => {
      // 返回 h() 需要的 svg 结构
      const path = mdiIcons[props.icon]
      if (!path) return null
      return {
        tag: 'svg',
        attrs: {
          viewBox: '0 0 24 24',
          role: 'img',
          'aria-hidden': 'true',
        },
        children: [{ tag: 'path', attrs: { d: path } }],
      }
    },
  }
}
