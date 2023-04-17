package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"uamemos/api"
	"uamemos/common"
	"uamemos/common/log"
	"uamemos/plugin/storage/s3"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// The max file size is 32MB.
	maxFileSize = 32 << 20
)

var fileKeyPattern = regexp.MustCompile(`\{[a-z]{1,9}\}`)

func (s *Service) registerResourceRoutes(rg *gin.RouterGroup) {
	rg.POST("/resource", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		resourceCreate := &api.ResourceCreate{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(resourceCreate); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post resource request")
			return
		}

		resourceCreate.CreatorID = userID
		// Only allow those external links with http prefix.
		if resourceCreate.ExternalLink != "" && !strings.HasPrefix(resourceCreate.ExternalLink, "http") {
			ctx.String(http.StatusBadRequest, "Invalid external link")
			return
		}

		resource, err := s.Store.CreateResource(ctx, resourceCreate)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create resource")
			return
		}
		if err := s.createResourceCreateActivity(ctx, resource); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(resource))
	})

	rg.POST("/resource/blob", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		if err := ctx.Request.ParseMultipartForm(maxFileSize); err != nil {
			ctx.String(http.StatusBadRequest, "Upload file overload max size")
			return
		}

		file, err := ctx.FormFile("file")
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to get uploading file")
			return
		}
		if file == nil {
			ctx.String(http.StatusBadRequest, "Upload file not found")
			return
		}

		filetype := file.Header.Get("Content-Type")
		size := file.Size
		sourceFile, err := file.Open()
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to open file")
			return
		}
		defer sourceFile.Close()

		var resourceCreate *api.ResourceCreate
		systemSettingStorageServiceID, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{Name: api.SystemSettingStorageServiceIDName})
		if err != nil && common.ErrorCode(err) != common.NotFound {
			ctx.String(http.StatusInternalServerError, "Failed to find storage")
			return
		}
		storageServiceID := api.DatabaseStorage
		if systemSettingStorageServiceID != nil {
			err = json.Unmarshal([]byte(systemSettingStorageServiceID.Value), &storageServiceID)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to unmarshal storage service id")
				return
			}
		}
		if storageServiceID == api.DatabaseStorage {
			fileBytes, err := io.ReadAll(sourceFile)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to read file")
				return
			}
			resourceCreate = &api.ResourceCreate{
				CreatorID: userID,
				Filename:  file.Filename,
				Type:      filetype,
				Size:      size,
				Blob:      fileBytes,
			}
		} else if storageServiceID == api.LocalStorage {
			systemSettingLocalStoragePath, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{Name: api.SystemSettingLocalStoragePathName})
			if err != nil && common.ErrorCode(err) != common.NotFound {
				ctx.String(http.StatusInternalServerError, "Failed to find local storage path setting")
				return
			}
			localStoragePath := ""
			if systemSettingLocalStoragePath != nil {
				err = json.Unmarshal([]byte(systemSettingLocalStoragePath.Value), &localStoragePath)
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to unmarshal local storage path setting")
					return
				}
			}
			filePath := localStoragePath
			if !strings.Contains(filePath, "{filename}") {
				filePath = path.Join(filePath, "{filename}")
			}
			filePath = path.Join(s.Profile.Data, replacePathTemplate(filePath, file.Filename))
			dir, filename := filepath.Split(filePath)
			if err = os.MkdirAll(dir, os.ModePerm); err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to create directory")
				return
			}
			dst, err := os.Create(filePath)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to create file")
				return
			}
			defer dst.Close()
			_, err = io.Copy(dst, sourceFile)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to copy file")
				return
			}

			resourceCreate = &api.ResourceCreate{
				CreatorID:    userID,
				Filename:     filename,
				Type:         filetype,
				Size:         size,
				InternalPath: filePath,
			}
		} else {
			storage, err := s.Store.FindStorage(ctx, &api.StorageFind{ID: &storageServiceID})
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to find storage")
				return
			}

			if storage.Type == api.StorageS3 {
				s3Config := storage.Config.S3Config
				s3Client, err := s3.NewClient(ctx, &s3.Config{
					AccessKey: s3Config.AccessKey,
					SecretKey: s3Config.SecretKey,
					EndPoint:  s3Config.EndPoint,
					Region:    s3Config.Region,
					Bucket:    s3Config.Bucket,
					URLPrefix: s3Config.URLPrefix,
					URLSuffix: s3Config.URLSuffix,
				})
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to new s3 client")
					return
				}

				filePath := s3Config.Path
				if !strings.Contains(filePath, "{filename}") {
					filePath = path.Join(filePath, "{filename}")
				}
				filePath = replacePathTemplate(filePath, file.Filename)
				_, filename := filepath.Split(filePath)
				link, err := s3Client.UploadFile(ctx, filePath, filetype, sourceFile)
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to upload via s3 client")
					return
				}
				resourceCreate = &api.ResourceCreate{
					CreatorID:    userID,
					Filename:     filename,
					Type:         filetype,
					ExternalLink: link,
				}
			} else {
				ctx.String(http.StatusInternalServerError, "Unsupported storage type")
				return
			}
		}

		publicID := common.GenUUID()
		resourceCreate.PublicID = publicID
		resource, err := s.Store.CreateResource(ctx, resourceCreate)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create resource")
			return
		}
		if err := s.createResourceCreateActivity(ctx, resource); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(resource))
	})

	rg.GET("/resource", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		resourceFind := &api.ResourceFind{
			CreatorID: &userID,
		}
		if limit, err := strconv.Atoi(ctx.Query("limit")); err == nil {
			resourceFind.Limit = &limit
		}
		if offset, err := strconv.Atoi(ctx.Query("offset")); err == nil {
			resourceFind.Offset = &offset
		}

		list, err := s.Store.FindResourceList(ctx, resourceFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch resource list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(list))
	})

	rg.PATCH("/resource/:resourceId", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		resourceID, err := strconv.Atoi(ctx.Param("resourceId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("resourceId")))
			return
		}

		resourceFind := &api.ResourceFind{
			ID: &resourceID,
		}
		resource, err := s.Store.FindResource(ctx, resourceFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find resource")
			return
		}
		if resource.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		currentTs := time.Now().Unix()
		resourcePatch := &api.ResourcePatch{
			UpdatedTs: &currentTs,
		}
		if err := json.NewDecoder(ctx.Request.Body).Decode(resourcePatch); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted patch resource request")
			return
		}

		if resourcePatch.ResetPublicID != nil && *resourcePatch.ResetPublicID {
			publicID := common.GenUUID()
			resourcePatch.PublicID = &publicID
		}

		resourcePatch.ID = resourceID
		resource, err = s.Store.PatchResource(ctx, resourcePatch)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to patch resource")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(resource))
	})

	rg.DELETE("/resource/:resourceId", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		resourceID, err := strconv.Atoi(ctx.Param("resourceId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("resourceId")))
			return
		}

		resource, err := s.Store.FindResource(ctx, &api.ResourceFind{
			ID:        &resourceID,
			CreatorID: &userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find resource")
			return
		}
		if resource.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		if resource.InternalPath != "" {
			err := os.Remove(resource.InternalPath)
			if err != nil {
				log.Warn(fmt.Sprintf("failed to delete local file with path %s", resource.InternalPath), zap.Error(err))
			}
		}

		resourceDelete := &api.ResourceDelete{
			ID: resourceID,
		}
		if err := s.Store.DeleteResource(ctx, resourceDelete); err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Resource ID not found: %d", resourceID))
				return
			}
			ctx.String(http.StatusInternalServerError, "Failed to delete resource")
			return
		}
		ctx.JSON(http.StatusOK, true)
	})
}

func (s *Service) registerResourcePublicRoutes(rg *gin.RouterGroup) {
	// (DEPRECATED) use /r/:resourceId/:publicId/:filename instead.
	rg.GET("/r/:resourceId/:publicId", func(ctx *gin.Context) {

		resourceID, err := strconv.Atoi(ctx.Param("resourceId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("resourceId")))
			return
		}
		publicID, err := url.QueryUnescape(ctx.Param("publicId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("publicID is invalid: %s", ctx.Param("publicId")))
			return
		}
		resourceFind := &api.ResourceFind{
			ID:       &resourceID,
			PublicID: &publicID,
			GetBlob:  true,
		}
		resource, err := s.Store.FindResource(ctx, resourceFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to find resource by ID: %v", resourceID))
			return
		}

		if resource.InternalPath != "" {
			src, err := os.Open(resource.InternalPath)
			if err != nil {
				ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to open the local resource: %s", resource.InternalPath))
				return
			}
			defer src.Close()
			ctx.Header("Cache-Control", "max-age=31536000, immutable")
			ctx.Header("Content-Security-Policy", "default-src 'self'")
			resourceType := strings.ToLower(resource.Type)
			ctx.Header("Content-Type", resourceType)
			ctx.File(resource.InternalPath)
		}
		ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to open the local resource: %s", resource.InternalPath))
	})

	rg.GET("/r/:resourceId/:publicId/:filename", func(ctx *gin.Context) {

		resourceID, err := strconv.Atoi(ctx.Param("resourceId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("resourceId")))
			return
		}
		publicID, err := url.QueryUnescape(ctx.Param("publicId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("publicID is invalid: %s", ctx.Param("publicId")))
			return
		}
		filename, err := url.QueryUnescape(ctx.Param("filename"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("filename is invalid: %s", ctx.Param("filename")))
			return
		}
		resourceFind := &api.ResourceFind{
			ID:       &resourceID,
			PublicID: &publicID,
			Filename: &filename,
			GetBlob:  true,
		}
		resource, err := s.Store.FindResource(ctx, resourceFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to find resource by ID: %v", resourceID))
			return
		}

		if resource.InternalPath != "" {
			src, err := os.Open(resource.InternalPath)
			if err != nil {
				ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to open the local resource: %s", resource.InternalPath))
				return
			}
			defer src.Close()
			ctx.Header("Cache-Control", "max-age=31536000, immutable")
			ctx.Header("Content-Security-Policy", "default-src 'self'")
			resourceType := strings.ToLower(resource.Type)
			ctx.Header("Content-Type", resourceType)
			ctx.File(resource.InternalPath)
		}
		ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to open the local resource: %s", resource.InternalPath))
	})
}

func (s *Service) createResourceCreateActivity(ctx *gin.Context, resource *api.Resource) error {

	payload := api.ActivityResourceCreatePayload{
		Filename: resource.Filename,
		Type:     resource.Type,
		Size:     resource.Size,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: resource.CreatorID,
		Type:      api.ActivityResourceCreate,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}

func replacePathTemplate(path string, filename string) string {
	t := time.Now()
	path = fileKeyPattern.ReplaceAllStringFunc(path, func(s string) string {
		switch s {
		case "{filename}":
			return filename
		case "{timestamp}":
			return fmt.Sprintf("%d", t.Unix())
		case "{year}":
			return fmt.Sprintf("%d", t.Year())
		case "{month}":
			return fmt.Sprintf("%02d", t.Month())
		case "{day}":
			return fmt.Sprintf("%02d", t.Day())
		case "{hour}":
			return fmt.Sprintf("%02d", t.Hour())
		case "{minute}":
			return fmt.Sprintf("%02d", t.Minute())
		case "{second}":
			return fmt.Sprintf("%02d", t.Second())
		}
		return s
	})
	return path
}
