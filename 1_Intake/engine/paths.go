package intake

import "blackfactory/pkg/bfcore"

// SanitizeAffectedPaths delega en bfcore.SanitizeRelativePaths conservando las opciones exactas
// de 1_Intake (archivos concretos, division por comas omitiendo vacios y deduplicacion sensible a mayusculas).
func SanitizeAffectedPaths(rawPaths []string, repoRoot string) ([]string, string) {
	return bfcore.SanitizeRelativePaths(rawPaths, repoRoot, bfcore.PathSanitizeOpts{
		AllowDir:       false,
		SplitCommas:    true,
		FoldCaseDedupe: false,
	})
}

// CountRootModules delega en bfcore.CountRootModules para extraer los modulos raiz distintos.
func CountRootModules(paths []string) []string {
	return bfcore.CountRootModules(paths)
}
