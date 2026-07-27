//go:build !darwin

package dockicon

// Materialize 非 macOS 平台为 no-op：直接返回原内核 exe，不做任何图标定制。
func (r *Resolver) Materialize(profileId, sourceExe, pngPath, displayName string) (string, error) {
	return sourceExe, nil
}
