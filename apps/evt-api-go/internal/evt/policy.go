package evt

const (
	MaxFilesPerInit  = 3
	MaxFileSizeBytes = 5 * 1024 * 1024 // 5MB
)

// Keep this tight for now. Add as needed.
var AllowedMimeTypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
}
