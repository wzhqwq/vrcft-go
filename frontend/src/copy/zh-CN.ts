export const copy = {
  navigation: {
    overview: '概览',
    plugins: '插件',
    settings: '设置',
    diagnostics: '诊断',
  },
  actions: {
    save: '保存',
    cancel: '取消',
    retry: '重试',
    copy: '复制',
  },
  settings: {
    savedRestart: '已保存，将在重启后生效',
  },
  problem: {
    validation: {
      title: '输入内容需要调整',
      defaultDetail: '请检查输入后重试。',
    },
    conflict: {
      title: '数据已在其他位置更新',
      defaultDetail: '当前数据已更新，请重新加载后再试。',
      detailWithRevision: (revision: number) => `当前版本已更新到修订 ${revision}，请重新加载后再试。`,
    },
    notFound: {
      title: '未找到所需内容',
      defaultDetail: '未找到所需内容。',
    },
    unavailable: {
      title: '当前功能暂不可用',
      defaultDetail: '请稍后重试，或检查应用是否仍在运行。',
    },
    unsupportedPlatform: {
      title: '当前平台暂不受支持',
      defaultDetail: '此功能当前仅支持受支持的平台环境。',
    },
    timeout: {
      title: '操作响应超时',
      defaultDetail: '请求处理时间过长，请重试。',
    },
    internal: {
      title: '应用发生内部错误',
      defaultDetail: '应用返回了受限错误信息，请复制诊断信息后重试。',
    },
    unknown: {
      title: '发生未知错误',
      defaultDetail: '应用返回了未识别的问题类型。',
    },
  },
} as const

export type Copy = typeof copy
