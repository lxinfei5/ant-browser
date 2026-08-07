/** Official Chrome for Testing (GoogleChromeLabs) — stock Chromium builds. */
export const CHROME_FOR_TESTING_DASHBOARD_URL = 'https://googlechromelabs.github.io/chrome-for-testing/'
export const CHROME_FOR_TESTING_LAST_KNOWN_GOOD_URL =
  'https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json'

/** @deprecated Use CHROME_FOR_TESTING_DASHBOARD_URL. Kept for any residual imports. */
export const FINGERPRINT_CHROMIUM_RELEASES_URL = CHROME_FOR_TESTING_DASHBOARD_URL

type CoreDownloadPlatform = 'windows' | 'linux' | 'darwin' | ''
type CoreDownloadArch = 'amd64' | 'arm64' | ''

export interface CoreDownloadRuntimeEnvironment {
  platform?: string
  arch?: string
}

export interface CoreDownloadTarget {
  platform: CoreDownloadPlatform
  arch: CoreDownloadArch
  label: string
}

export interface CoreDownloadRecommendation {
  target: CoreDownloadTarget
  releaseTag: string
  assetName: string
  downloadUrl: string
  releasesUrl: string
  namePlaceholder: string
}

interface ChromeForTestingDownload {
  platform?: string
  url?: string
}

interface ChromeForTestingChannel {
  version?: string
  downloads?: {
    chrome?: ChromeForTestingDownload[]
  }
}

interface ChromeForTestingResponse {
  channels?: {
    Stable?: ChromeForTestingChannel
    Beta?: ChromeForTestingChannel
    Dev?: ChromeForTestingChannel
    Canary?: ChromeForTestingChannel
  }
}

function normalizePlatform(value: string | undefined): CoreDownloadPlatform {
  const text = (value || '').trim().toLowerCase()
  if (text.includes('win')) return 'windows'
  if (text.includes('linux')) return 'linux'
  if (text.includes('darwin') || text.includes('mac')) return 'darwin'
  return ''
}

function normalizeArch(value: string | undefined): CoreDownloadArch {
  const text = (value || '').trim().toLowerCase()
  if (text === 'amd64' || text === 'x64' || text === 'x86_64') return 'amd64'
  if (text === 'arm64' || text === 'aarch64') return 'arm64'
  if (hasAny(text, ['arm64', 'aarch64'])) return 'arm64'
  if (hasAny(text, ['amd64', 'x64', 'x86_64', 'win64', 'wow64'])) return 'amd64'
  return ''
}

function inferNavigatorEnvironment(): CoreDownloadRuntimeEnvironment {
  if (typeof navigator === 'undefined') return {}
  const nav = navigator as Navigator & { userAgentData?: { platform?: string; architecture?: string } }
  const platform = nav.userAgentData?.platform || navigator.platform || ''
  const userAgent = navigator.userAgent || ''
  const archSource = `${nav.userAgentData?.architecture || ''} ${navigator.platform || ''} ${userAgent}`
  return { platform, arch: archSource }
}

export function resolveCoreDownloadTarget(env: CoreDownloadRuntimeEnvironment | null | undefined): CoreDownloadTarget {
  const fallback = inferNavigatorEnvironment()
  const platform = normalizePlatform(env?.platform || fallback.platform)
  const arch = normalizeArch(env?.arch || fallback.arch)
  return { platform, arch, label: formatTargetLabel(platform, arch) }
}

function formatTargetLabel(platform: CoreDownloadPlatform, arch: CoreDownloadArch): string {
  const platformLabel = platform === 'windows' ? 'Windows' : platform === 'linux' ? 'Linux' : platform === 'darwin' ? 'macOS' : '未知系统'
  const archLabel = arch || '未知架构'
  return `${platformLabel} / ${archLabel}`
}

function hasAny(text: string, keywords: string[]): boolean {
  return keywords.some(keyword => text.includes(keyword))
}

/** Map ProfilePool target → Chrome for Testing platform id. */
function chromeForTestingPlatform(target: CoreDownloadTarget): string {
  if (target.platform === 'windows' && target.arch === 'amd64') return 'win64'
  if (target.platform === 'linux' && target.arch === 'amd64') return 'linux64'
  if (target.platform === 'darwin' && target.arch === 'arm64') return 'mac-arm64'
  if (target.platform === 'darwin' && target.arch === 'amd64') return 'mac-x64'
  return ''
}

function pickChromeDownload(channel: ChromeForTestingChannel | undefined, cftPlatform: string): ChromeForTestingDownload | null {
  const downloads = channel?.downloads?.chrome || []
  return downloads.find(item => (item.platform || '').toLowerCase() === cftPlatform && !!item.url) || null
}

export async function fetchCoreDownloadRecommendation(
  env: CoreDownloadRuntimeEnvironment | null | undefined,
  signal?: AbortSignal,
): Promise<CoreDownloadRecommendation | null> {
  const target = resolveCoreDownloadTarget(env)
  const cftPlatform = chromeForTestingPlatform(target)
  if (!cftPlatform) return null

  const response = await fetch(CHROME_FOR_TESTING_LAST_KNOWN_GOOD_URL, { signal })
  if (!response.ok) {
    throw new Error(`Chrome for Testing 元数据请求失败: ${response.status}`)
  }
  const payload = await response.json() as ChromeForTestingResponse
  const channel = payload.channels?.Stable
  const download = pickChromeDownload(channel, cftPlatform)
  if (!download?.url) return null

  const version = (channel?.version || '').trim()
  const majorVersion = version.split('.')[0] || ''
  const assetName = download.url.split('/').pop() || `chrome-${cftPlatform}.zip`

  return {
    target,
    releaseTag: version,
    assetName,
    downloadUrl: download.url,
    releasesUrl: CHROME_FOR_TESTING_DASHBOARD_URL,
    namePlaceholder: majorVersion ? `例如: chrome-${majorVersion}` : '例如: chrome-stable',
  }
}
