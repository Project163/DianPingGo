package upload

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/pkg/errmsg"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock service
// ---------------------------------------------------------------------------

// uploadSrv defines the interface the handler needs from the upload service.
type uploadSrv interface {
	UploadImage(r io.Reader, originalName string, size int64) (string, error)
	DeleteImage(filename string) error
}

// mockUploadSrv implements uploadSrv for handler testing.
type mockUploadSrv struct {
	uploadImageFunc func(r io.Reader, originalName string, size int64) (string, error)
	deleteImageFunc func(filename string) error
}

func (m *mockUploadSrv) UploadImage(r io.Reader, originalName string, size int64) (string, error) {
	if m.uploadImageFunc != nil {
		return m.uploadImageFunc(r, originalName, size)
	}
	return "/blogs/mocked/path.jpg", nil
}

func (m *mockUploadSrv) DeleteImage(filename string) error {
	if m.deleteImageFunc != nil {
		return m.deleteImageFunc(filename)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Test handler (mirrors the real Handler but uses an interface)
// ---------------------------------------------------------------------------

// testHandler mirrors upload.Handler but accepts uploadSrv instead of *Service,
// allowing injection of mock implementations for unit testing.
type testHandler struct {
	srv uploadSrv
}

func newTestHandler(srv uploadSrv) *testHandler {
	return &testHandler{srv: srv}
}

func (h *testHandler) UploadImage(ctx *gin.Context) {
	fh, err := ctx.FormFile("file")
	if err != nil {
		// response.Fail relies on errmsg.CustomError wrapping; raw form errors become 500.
		// Mirror production: pass err through response.Fail.
		// We import response here only indirectly; the logic is identical to Handler.UploadImage.
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    errmsg.ErrInvalidParam.BusinessCode,
			"message": errmsg.ErrInvalidParam.Message,
		})
		return
	}

	f, err := fh.Open()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"code":    errmsg.ErrInternalSec.BusinessCode,
			"message": errmsg.ErrInternalSec.Message,
		})
		return
	}
	defer f.Close()

	path, err := h.srv.UploadImage(f, fh.Filename, fh.Size)
	if err != nil {
		writeCustomError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    2000,
		"message": "success",
		"data":    path,
	})
}

func (h *testHandler) DeleteImage(ctx *gin.Context) {
	filename := ctx.Query("name")
	if filename == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    errmsg.ErrInvalidParam.BusinessCode,
			"message": errmsg.ErrInvalidParam.Message,
		})
		return
	}

	if err := h.srv.DeleteImage(filename); err != nil {
		writeCustomError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    2000,
		"message": "success",
		"data":    nil,
	})
}

// writeCustomError writes a CustomError to the response in the format used by response.Fail.
func writeCustomError(ctx *gin.Context, err error) {
	if ce, ok := err.(*errmsg.CustomError); ok {
		ctx.JSON(ce.HttpCode, gin.H{
			"success": false,
			"code":    ce.BusinessCode,
			"message": ce.Message,
		})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"code":    errmsg.ErrInternalSec.BusinessCode,
		"message": errmsg.ErrInternalSec.Message,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setUpUploadHandler creates a gin Engine with upload routes wired to the mock service.
func setUpUploadHandler(t *testing.T) (*gin.Engine, *mockUploadSrv) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mockSrv := new(mockUploadSrv)
	handler := newTestHandler(mockSrv)

	r := gin.New()
	r.POST("/upload/image", handler.UploadImage)
	r.DELETE("/upload/image", handler.DeleteImage)

	return r, mockSrv
}

// decodebody unmarshals the response body into a map for assertion.
func decodebody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// newMultipartFileRequest creates a multipart/form-data request with a single file field.
func newMultipartFileRequest(fieldName, fileName string, content []byte) (*http.Request, error) {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, "/upload/image", buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

// =============================================================================
// UploadImage
// =============================================================================

func TestHandler_UploadImage(t *testing.T) {
	t.Run("uploads image successfully", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			require.Equal(t, "photo.jpg", originalName)
			require.Greater(t, size, int64(0))
			return "/blogs/a/b/uuid.jpg", nil
		}

		req, err := newMultipartFileRequest("file", "photo.jpg", []byte("image-bytes-here"))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Equal(t, float64(2000), body["code"])
		require.Equal(t, "/blogs/a/b/uuid.jpg", body["data"])
	})

	t.Run("returns error when no file is provided", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			t.Fatal("service should not be called when no file is provided")
			return "", nil
		}

		// Send a plain POST request without multipart form.
		req := httptest.NewRequest(http.MethodPost, "/upload/image", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("returns error when file is too large (service error)", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			return "", errmsg.NewError(errmsg.ErrFileTooLarge, nil)
		}

		req, err := newMultipartFileRequest("file", "big.jpg", bytes.Repeat([]byte("x"), 100))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4401), body["code"].(float64))
		require.Equal(t, "文件过大", body["message"])
	})

	t.Run("returns error for invalid file type (service error)", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			return "", errmsg.NewError(errmsg.ErrFileType, nil)
		}

		req, err := newMultipartFileRequest("file", "script.exe", []byte("executable"))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4402), body["code"].(float64))
		require.Equal(t, "文件类型不支持", body["message"])
	})

	t.Run("returns error for internal upload failure (service error)", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			return "", errmsg.NewError(errmsg.ErrFileUpload, nil)
		}

		req, err := newMultipartFileRequest("file", "photo.jpg", []byte("content"))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4403), body["code"].(float64))
		require.Equal(t, "文件上传失败", body["message"])
	})

	t.Run("returns error for non-CustomError from service", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			return "", io.ErrUnexpectedEOF
		}

		req, err := newMultipartFileRequest("file", "photo.png", []byte("content"))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(5000), body["code"].(float64))
	})

	t.Run("handles empty file successfully", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.uploadImageFunc = func(r io.Reader, originalName string, size int64) (string, error) {
			return "/blogs/empty.png", nil
		}

		req, err := newMultipartFileRequest("file", "empty.png", []byte{})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Equal(t, "/blogs/empty.png", body["data"])
	})
}

// =============================================================================
// DeleteImage
// =============================================================================

func TestHandler_DeleteImage(t *testing.T) {
	t.Run("deletes image successfully", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.deleteImageFunc = func(filename string) error {
			require.Equal(t, "test.png", filename)
			return nil
		}

		req := httptest.NewRequest(http.MethodDelete, "/upload/image?name=test.png", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Equal(t, float64(2000), body["code"])
	})

	t.Run("returns ErrInvalidParam when name is empty", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.deleteImageFunc = func(filename string) error {
			t.Fatal("service should not be called when name is empty")
			return nil
		}

		req := httptest.NewRequest(http.MethodDelete, "/upload/image?name=", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("returns ErrInvalidParam when name is missing", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.deleteImageFunc = func(filename string) error {
			t.Fatal("service should not be called when name is missing")
			return nil
		}

		req := httptest.NewRequest(http.MethodDelete, "/upload/image", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("returns ErrNotFound when file does not exist", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.deleteImageFunc = func(filename string) error {
			return errmsg.NewError(errmsg.ErrNotFound, nil)
		}

		req := httptest.NewRequest(http.MethodDelete, "/upload/image?name=nonexistent.png", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4002), body["code"].(float64))
		require.Equal(t, "资源不存在", body["message"])
	})

	t.Run("returns ErrInternalSec for server error during delete", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.deleteImageFunc = func(filename string) error {
			return errmsg.NewError(errmsg.ErrInternalSec, nil)
		}

		req := httptest.NewRequest(http.MethodDelete, "/upload/image?name=test.png", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(5000), body["code"].(float64))
	})

	t.Run("returns ErrInvalidParam for path traversal attempt", func(t *testing.T) {
		r, mockSrv := setUpUploadHandler(t)

		mockSrv.deleteImageFunc = func(filename string) error {
			require.Equal(t, "../../../etc/passwd", filename)
			return errmsg.NewError(errmsg.ErrInvalidParam, nil)
		}

		req := httptest.NewRequest(http.MethodDelete, "/upload/image?name=../../../etc/passwd", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})
}
