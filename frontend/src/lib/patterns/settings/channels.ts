export interface ChannelOption {
  id: string
  label: string
  category: string
}

const eyeChannels = [
  ['eye.left_gaze_x', 'Left Gaze X'],
  ['eye.left_gaze_y', 'Left Gaze Y'],
  ['eye.right_gaze_x', 'Right Gaze X'],
  ['eye.right_gaze_y', 'Right Gaze Y'],
  ['eye.left_openness', 'Left Openness'],
  ['eye.right_openness', 'Right Openness'],
  ['eye.left_pupil_diameter', 'Left Pupil Diameter'],
  ['eye.right_pupil_diameter', 'Right Pupil Diameter'],
  ['eye.left_pupil_dilation', 'Left Pupil Dilation'],
  ['eye.right_pupil_dilation', 'Right Pupil Dilation'],
] as const

const expressionNames = [
  'EyeSquintRight', 'EyeSquintLeft',
  'BrowPinchRight', 'BrowPinchLeft', 'BrowLowererRight', 'BrowLowererLeft', 'BrowInnerUpRight', 'BrowInnerUpLeft', 'BrowOuterUpRight', 'BrowOuterUpLeft',
  'NoseSneerRight', 'NoseSneerLeft', 'NasalDilationRight', 'NasalDilationLeft', 'NasalConstrictRight', 'NasalConstrictLeft',
  'CheekSquintRight', 'CheekSquintLeft', 'CheekPuffSuckRight', 'CheekPuffSuckLeft',
  'JawOpen', 'MouthClosed', 'JawX', 'JawZ', 'JawClench', 'JawMandibleRaise',
  'LipSuckUpperRight', 'LipSuckUpperLeft', 'LipSuckLowerRight', 'LipSuckLowerLeft', 'LipSuckCornerRight', 'LipSuckCornerLeft',
  'LipFunnelUpperRight', 'LipFunnelUpperLeft', 'LipFunnelLowerRight', 'LipFunnelLowerLeft',
  'LipPuckerUpperRight', 'LipPuckerUpperLeft', 'LipPuckerLowerRight', 'LipPuckerLowerLeft',
  'MouthUpperUpRight', 'MouthUpperUpLeft', 'MouthLowerDownRight', 'MouthLowerDownLeft', 'MouthUpperDeepenRight', 'MouthUpperDeepenLeft', 'MouthUpperX', 'MouthLowerX',
  'MouthCornerPullRight', 'MouthCornerPullLeft', 'MouthCornerSlantRight', 'MouthCornerSlantLeft', 'MouthDimpleRight', 'MouthDimpleLeft',
  'MouthFrownRight', 'MouthFrownLeft', 'MouthStretchRight', 'MouthStretchLeft', 'MouthRaiserUpper', 'MouthRaiserLower',
  'MouthPressRight', 'MouthPressLeft', 'MouthTightenerRight', 'MouthTightenerLeft',
  'TongueOut', 'TongueX', 'TongueY', 'TongueRoll', 'TongueArchY', 'TongueShape', 'TongueTwistRight', 'TongueTwistLeft',
  'SoftPalateClose', 'ThroatSwallow', 'NeckFlexRight', 'NeckFlexLeft',
] as const

const categoryOrder = ['眼动', '眼部表情', '眉部', '鼻部', '面颊', '下颌', '嘴部', '舌部', '颈部与其他'] as const

function expressionCategory(name: string): string {
  if (name.startsWith('Eye')) return '眼部表情'
  if (name.startsWith('Brow')) return '眉部'
  if (name.startsWith('Nose') || name.startsWith('Nasal')) return '鼻部'
  if (name.startsWith('Cheek')) return '面颊'
  if (name.startsWith('Jaw')) return '下颌'
  if (name.startsWith('Mouth') || name.startsWith('Lip')) return '嘴部'
  if (name.startsWith('Tongue')) return '舌部'
  return '颈部与其他'
}

function readableName(name: string): string {
  return name.replace(/([a-z0-9])([A-Z])/g, '$1 $2')
}

export const channels: readonly ChannelOption[] = [
  ...eyeChannels.map(([id, label]) => ({id, label, category: '眼动'})),
  ...expressionNames.map((name) => ({id: `expression:${name}`, label: readableName(name), category: expressionCategory(name)})),
]

const labels = new Map(channels.map((channel) => [channel.id, channel.label]))

export const channelGroups = categoryOrder.map((category) => ({
  category,
  channels: channels.filter((channel) => channel.category === category),
})).filter((group) => group.channels.length > 0)

export function channelLabel(id: string): string {
  return labels.get(id) ?? id
}
