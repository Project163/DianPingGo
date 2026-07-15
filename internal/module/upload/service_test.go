package upload

import (
	"bytes"
	"dianping/internal/config"
	"dianping/pkg/errmsg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// setUpUploadService creates a Service and a temp directory for file operations.
// It also sets config.GlobalConfig with permissive upload settings for testing.
func setUpUploadService(t *testing.T) (*Service, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "upload-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(tmpDir))
	})

	config.GlobalConfig = &config.Config{
		Upload: config.UploadConfig{
			Dir:          tmpDir,
			MaxSize:      1024 * 1024, // 1 MiB
			AllowedTypes: []string{".jpg", ".jpeg", ".png", ".gif", ".webp"},
		},
	}

	return NewService(), tmpDir
}

// =============================================================================
// UploadImage
// =============================================================================

func TestService_UploadImage(t *testing.T) {
	t.Run("uploads a valid image file successfully", func(t *testing.T) {
		service, tmpDir := setUpUploadService(t)

		content := []byte("fake-image-content-abc123")
		reader := bytes.NewReader(content)
		path, err := service.UploadImage(reader, "photo.jpg", int64(len(content)))

		require.NoError(t, err)
		require.NotEmpty(t, path)
		require.True(t, strings.HasPrefix(path, "/blogs/"))
		require.True(t, strings.HasSuffix(path, ".jpg"))

		// Verify the file was actually created on disk.
		fullPath := filepath.Join(tmpDir, path)
		_, statErr := os.Stat(fullPath)
		require.NoError(t, statErr, "expected file to exist at %s", fullPath)

		// Verify file content.
		saved, readErr := os.ReadFile(fullPath)
		require.NoError(t, readErr)
		require.Equal(t, content, saved)
	})

	t.Run("uploads png file successfully", func(t *testing.T) {
		service, tmpDir := setUpUploadService(t)

		content := []byte("png-binary-content")
		reader := bytes.NewReader(content)
		path, err := service.UploadImage(reader, "avatar.png", int64(len(content)))

		require.NoError(t, err)
		require.True(t, strings.HasSuffix(path, ".png"))

		fullPath := filepath.Join(tmpDir, path)
		saved, readErr := os.ReadFile(fullPath)
		require.NoError(t, readErr)
		require.Equal(t, content, saved)
	})

	t.Run("rejects file exceeding max size", func(t *testing.T) {
		service, _ := setUpUploadService(t)

		// Set a very small max size to force rejection.
		config.GlobalConfig.Upload.MaxSize = 100
		largeContent := bytes.Repeat([]byte("x"), 200)
		reader := bytes.NewReader(largeContent)

		path, err := service.UploadImage(reader, "big.jpg", 200)
		require.Error(t, err)
		require.Empty(t, path)
		require.Equal(t, &errmsg.ErrFileTooLarge, err)
	})

	t.Run("rejects file with invalid extension", func(t *testing.T) {
		service, _ := setUpUploadService(t)

		content := []byte("not-a-real-exe")
		reader := bytes.NewReader(content)

		path, err := service.UploadImage(reader, "malware.exe", int64(len(content)))
		require.Error(t, err)
		require.Empty(t, path)
		require.Equal(t, &errmsg.ErrFileType, err)
	})

	t.Run("rejects file with no extension", func(t *testing.T) {
		service, _ := setUpUploadService(t)

		content := []byte("no-extension-content")
		reader := bytes.NewReader(content)

		path, err := service.UploadImage(reader, "noextension", int64(len(content)))
		require.Error(t, err)
		require.Empty(t, path)
		require.Equal(t, &errmsg.ErrFileType, err)
	})

	t.Run("handles uppercase extension", func(t *testing.T) {
		service, tmpDir := setUpUploadService(t)

		content := []byte("uppercase-ext")
		reader := bytes.NewReader(content)
		path, err := service.UploadImage(reader, "PHOTO.JPG", int64(len(content)))

		require.NoError(t, err)
		require.True(t, strings.HasSuffix(path, ".jpg"))

		fullPath := filepath.Join(tmpDir, path)
		_, statErr := os.Stat(fullPath)
		require.NoError(t, statErr)
	})

	t.Run("handles empty file successfully", func(t *testing.T) {
		service, tmpDir := setUpUploadService(t)

		reader := bytes.NewReader([]byte{})
		path, err := service.UploadImage(reader, "empty.png", 0)

		require.NoError(t, err)
		require.NotEmpty(t, path)

		fullPath := filepath.Join(tmpDir, path)
		saved, readErr := os.ReadFile(fullPath)
		require.NoError(t, readErr)
		require.Empty(t, saved)
	})

	t.Run("handles zero size but large reader content (size check bypass)", func(t *testing.T) {
		service, _ := setUpUploadService(t)

		// Even if size passed is 0, the reader can provide any data.
		// The size check uses the parameter, not the reader length.
		config.GlobalConfig.Upload.MaxSize = 10
		content := bytes.Repeat([]byte("a"), 1024)
		reader := bytes.NewReader(content)

		path, err := service.UploadImage(reader, "large-but-zero-size.jpg", 0)
		require.NoError(t, err)
		require.NotEmpty(t, path)
	})
}

// =============================================================================
// DeleteImage
// =============================================================================

func TestService_DeleteImage(t *testing.T) {
	t.Run("deletes an existing file successfully", func(t *testing.T) {
		service, tmpDir := setUpUploadService(t)

		// Create a known file to delete.
		testPath := filepath.Join(tmpDir, "testfile.png")
		err := os.WriteFile(testPath, []byte("todelete"), 0644)
		require.NoError(t, err)

		// Delete using the relative path within the upload dir.
		err = service.DeleteImage("testfile.png")
		require.NoError(t, err)

		// Verify the file no longer exists.
		_, statErr := os.Stat(testPath)
		require.True(t, os.IsNotExist(statErr))
	})

	t.Run("returns ErrNotFound for non-existent file", func(t *testing.T) {
		service, _ := setUpUploadService(t)

		err := service.DeleteImage("nonexistent.png")
		require.Error(t, err)
		require.Equal(t, &errmsg.ErrNotFound, err)
	})

	t.Run("returns ErrInvalidParam for path containing ..", func(t *testing.T) {
		service, _ := setUpUploadService(t)

		err := service.DeleteImage("../../../etc/passwd")
		require.Error(t, err)
		require.Equal(t, &errmsg.ErrInvalidParam, err)
	})

	t.Run("returns ErrInvalidParam when path points to a directory", func(t *testing.T) {
		service, tmpDir := setUpUploadService(t)

		// Create a subdirectory.
		subDir := filepath.Join(tmpDir, "mydir")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		err = service.DeleteImage("mydir")
		require.Error(t, err)
		var myErr *errmsg.CustomError
		require.ErrorAs(t, err, &myErr)
		require.Equal(t, errmsg.ErrInvalidParam.BusinessCode, myErr.BusinessCode)
		require.Contains(t, myErr.Error(), "is a directory, not a file")
	})
}

// =============================================================================
// isAllowedExtension
// =============================================================================

func TestService_isAllowedExtension(t *testing.T) {
	service, _ := setUpUploadService(t)

	t.Run("returns true for allowed extensions", func(t *testing.T) {
		allowed := []string{".jpg", ".png", ".gif"}
		require.True(t, service.isAllowedExtension(".jpg", allowed))
		require.True(t, service.isAllowedExtension(".png", allowed))
		require.True(t, service.isAllowedExtension(".gif", allowed))
	})

	t.Run("returns false for disallowed extensions", func(t *testing.T) {
		allowed := []string{".jpg", ".png"}
		require.False(t, service.isAllowedExtension(".exe", allowed))
		require.False(t, service.isAllowedExtension("", allowed))
		require.False(t, service.isAllowedExtension(".JPG", allowed))
	})

	t.Run("returns false for empty allowed list", func(t *testing.T) {
		require.False(t, service.isAllowedExtension(".jpg", []string{}))
		require.False(t, service.isAllowedExtension(".jpg", nil))
	})
}
