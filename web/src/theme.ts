import type { GlobalThemeOverrides } from 'naive-ui'

// Clean & fresh enterprise dashboard. Lemon yellow stays the brand accent but is
// used sparingly; the canvas is a light, cool off-white with airy spacing,
// generous radii and soft hairlines instead of heavy borders/shadows.
const BRAND = '#F2C200'
const BRAND_HOVER = '#FFD60A'
const BRAND_PRESSED = '#D9AC00'
const INK = '#1F2933'

export const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: BRAND,
    primaryColorHover: BRAND_HOVER,
    primaryColorPressed: BRAND_PRESSED,
    primaryColorSuppl: BRAND,

    borderRadius: '10px',
    borderRadiusSmall: '8px',

    bodyColor: '#F6F8FB',
    cardColor: '#FFFFFF',
    borderColor: '#ECEEF2',
    dividerColor: '#EEF1F4',

    textColorBase: INK,
    textColor1: '#1F2933',
    textColor2: '#52606D',
    textColor3: '#9AA5B1',
    fontWeightStrong: '600',
  },
  Card: {
    borderRadius: '14px',
    color: '#FFFFFF',
    borderColor: '#EEF1F4',
  },
  Button: {
    // Lemon needs dark text for readability.
    textColorPrimary: INK,
    textColorHoverPrimary: INK,
    textColorPressedPrimary: INK,
    textColorFocusPrimary: INK,
    borderRadiusMedium: '10px',
    borderRadiusSmall: '8px',
    fontWeight: '500',
  },
  Layout: {
    siderColor: '#FFFFFF',
    headerColor: '#FFFFFF',
    color: '#F6F8FB',
  },
  Menu: {
    borderRadius: '10px',
    itemHeight: '44px',
    itemColorActive: 'rgba(242, 194, 0, 0.16)',
    itemColorActiveHover: 'rgba(242, 194, 0, 0.22)',
    itemTextColorActive: INK,
    itemTextColorActiveHover: INK,
    itemIconColorActive: '#C79A00',
    itemIconColorActiveHover: '#C79A00',
  },
  Input: {
    borderRadius: '10px',
    heightMedium: '38px',
  },
  DataTable: {
    thColor: '#FBFCFD',
    thTextColor: '#7B8794',
    thFontWeight: '600',
    borderColor: '#EEF1F4',
    tdColorHover: '#FAFBFC',
    borderRadius: '12px',
  },
  Tag: { borderRadius: '8px' },
  Statistic: { labelTextColor: '#7B8794' },
  // Light-gray tooltip instead of Naive's default near-black; dark ink text.
  Tooltip: { borderRadius: '8px', color: '#F3F4F6', textColor: '#1F2933' },
  Modal: { borderRadius: '16px' },
}
