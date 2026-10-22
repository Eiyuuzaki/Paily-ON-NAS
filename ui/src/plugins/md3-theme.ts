// ── Material Design 3 (Material You) 色板 ────────────────────────────────
// 采用 MD3 标准的 tonal palette 结构。品牌色为紫色系（Primary #6750A4）。
//
// MD3 的视觉分层不靠阴影，而靠 surface-container 色阶：
//   surface-container-lowest  ← 最低（卡片浮在上面时用最亮）
//   surface-container-low
//   surface-container
//   surface-container-high
//   surface-container-highest ← 最高
//
// 这样才有"Material You"的层次感，而不是 MD2 的投影堆叠。

export const md3Light = {
  dark: false,
  colors: {
    background: '#FEF7FF',
    surface: '#FEF7FF',
    'surface-bright': '#FEF7FF',
    'surface-light': '#F7F2FA',
    'surface-variant': '#E7E0EC',
    'on-surface-variant': '#49454F',

    'surface-container-lowest': '#FFFFFF',
    'surface-container-low': '#F7F2FA',
    'surface-container': '#F3EDF7',
    'surface-container-high': '#ECE6F0',
    'surface-container-highest': '#E6E0E9',

    primary: '#6750A4',
    'primary-darken-1': '#57458C',
    'on-primary': '#FFFFFF',
    'primary-container': '#EADDFF',
    'on-primary-container': '#21005D',

    secondary: '#625B71',
    'on-secondary': '#FFFFFF',
    'secondary-container': '#E8DEF8',
    'on-secondary-container': '#1D192B',

    tertiary: '#7D5260',
    'on-tertiary': '#FFFFFF',
    'tertiary-container': '#FFD8E4',
    'on-tertiary-container': '#31111D',

    error: '#B3261E',
    'on-error': '#FFFFFF',
    'error-container': '#F9DEDC',
    'on-error-container': '#410E0B',

    info: '#0061A4',
    'info-container': '#D1E4FF',
    success: '#2E6A3E',
    'success-container': '#C8F0D0',
    warning: '#7A5900',
    'warning-container': '#FFDF9E',

    outline: '#79747E',
    'outline-variant': '#CAC4D0',
  },
}

export const md3Dark = {
  dark: true,
  colors: {
    background: '#141218',
    surface: '#141218',
    'surface-bright': '#3B383E',
    'surface-light': '#1D1B20',
    'surface-variant': '#49454F',
    'on-surface-variant': '#CAC4D0',

    'surface-container-lowest': '#0F0D13',
    'surface-container-low': '#1D1B20',
    'surface-container': '#211F26',
    'surface-container-high': '#2B2930',
    'surface-container-highest': '#36343B',

    primary: '#D0BCFF',
    'primary-darken-1': '#B69DF8',
    'on-primary': '#381E72',
    'primary-container': '#4F378B',
    'on-primary-container': '#EADDFF',

    secondary: '#CCC2DC',
    'on-secondary': '#332D41',
    'secondary-container': '#4A4458',
    'on-secondary-container': '#E8DEF8',

    tertiary: '#EFB8C8',
    'on-tertiary': '#492532',
    'tertiary-container': '#633B48',
    'on-tertiary-container': '#FFD8E4',

    error: '#F2B8B5',
    'on-error': '#601410',
    'error-container': '#8C1D18',
    'on-error-container': '#F9DEDC',

    info: '#9ECAFF',
    'info-container': '#00497D',
    success: '#A6DDB2',
    'success-container': '#235231',
    warning: '#F5C443',
    'warning-container': '#5C4300',

    outline: '#938F99',
    'outline-variant': '#49454F',
  },
}
