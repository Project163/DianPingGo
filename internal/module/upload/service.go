package upload

import (
	"crypto/sha256"
	"dianping/internal/config"
	"dianping/pkg/errmsg"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

// UploadImage 上传图片，接受文件读取器、原始文件名和文件大小作为参数，返回保存路径和错误
func (s *Service) UploadImage(r io.Reader, originalName string, size int64) (string, error) {
	cfg := config.GlobalConfig.Upload
	if size > cfg.MaxSize {
		return "", errmsg.NewError(errmsg.ErrFileTooLarge, nil)
	}
	// 检查文件扩展名是否允许
	ext := strings.ToLower(filepath.Ext(originalName))
	if !s.isAllowedExtension(ext, cfg.AllowedTypes) {
		return "", errmsg.NewError(errmsg.ErrFileType, nil)
	}

	// 生成唯一的文件名和子目录结构
	// 目的是为了避免在同一目录下存储过多文件，导致文件系统性能下降
	// 以及防止用户传入危险的文件名(比如/etc/passwd)导致的安全问题
	id := uuid.New().String()
	hash := sha256.Sum256([]byte(id))
	d1 := hash[0] & 0xF
	d2 := (hash[0] >> 4) & 0xF
	subDir := filepath.Join(cfg.Dir, "blogs", fmt.Sprintf("%x", d1), fmt.Sprintf("%x", d2))

	if err := os.MkdirAll(subDir, 0755); err != nil {
		return "", errmsg.NewError(errmsg.ErrFileUpload, err)
	}

	savedName := fmt.Sprintf("%s%s", id, ext)
	savedPath := filepath.Join(subDir, savedName)

	dst, err := os.Create(savedPath)
	if err != nil {
		return "", errmsg.NewError(errmsg.ErrFileUpload, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, r); err != nil {
		return "", errmsg.NewError(errmsg.ErrFileUpload, err)
	}

	finalPath := fmt.Sprintf("/blogs/%x/%x/%s", d1, d2, savedName)
	return finalPath, nil
}

func (s *Service) DeleteImage(filename string) error {
	cfg := config.GlobalConfig.Upload
	cleanName := filepath.Clean(filename)
	if strings.Contains(cleanName, "..") {
		return errmsg.NewError(errmsg.ErrInvalidParam, nil)
	}

	fullPath := filepath.Join(cfg.Dir, cleanName)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errmsg.NewError(errmsg.ErrNotFound, nil)
		}
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	if info.IsDir() {
		return errmsg.NewError(errmsg.ErrInvalidParam, fmt.Errorf("path %s is a directory, not a file", fullPath))
	}
	if err := os.Remove(fullPath); err != nil {
		return errmsg.NewError(errmsg.ErrInternalSec, err)
	}
	return nil
}

func (s *Service) isAllowedExtension(ext string, allowedExts []string) bool {
	for _, allowed := range allowedExts {
		if ext == allowed {
			return true
		}
	}
	return false
}
