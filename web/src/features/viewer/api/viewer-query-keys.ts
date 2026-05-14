export const viewerKeys = {
	all: ['viewer'] as const,
	podSession: (podSessionID: string) =>
		[...viewerKeys.all, 'pod-session', podSessionID] as const,
	pvcs: (namespace: string) => [...viewerKeys.all, 'pvcs', namespace] as const,
	viewerSession: (viewerSessionID: string) =>
		[...viewerKeys.all, 'viewer-session', viewerSessionID] as const,
	viewerToken: (viewerSessionID: string) =>
		[...viewerKeys.all, 'viewer-token', viewerSessionID] as const,
}
