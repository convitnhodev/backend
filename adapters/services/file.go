package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/linxGnu/goseaweedfs"
	"github.com/seaweedfs/seaweedfs/weed/filer"

	"github.com/SeaCloudHub/backend/domain/file"
	"github.com/SeaCloudHub/backend/pkg/config"
)

type FileService struct {
	sw    *goseaweedfs.Seaweed
	filer *goseaweedfs.Filer
}

func NewFileService(cfg *config.Config) *FileService {
	sw, err := goseaweedfs.NewSeaweed(cfg.SeaweedFS.MasterServer,
		[]string{cfg.SeaweedFS.FilerServer}, 8096, http.DefaultClient)
	if err != nil {
		panic(err)
	}

	return &FileService{
		sw:    sw,
		filer: sw.Filers()[0],
	}
}

func (s *FileService) GetFile(ctx context.Context, filePath string) (*file.Entry, error) {
	query := url.Values{}
	query.Set("metadata", "true")

	header := map[string]string{
		"Accept": "application/json",
	}

	data, code, err := s.filer.Get(filePath, query, header)
	if err != nil {
		return nil, err
	}

	if code == http.StatusNotFound {
		return nil, file.ErrFileNotFound
	}

	if code != http.StatusOK {
		return nil, errors.New("failed to get file")
	}

	var entry filer.Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	return mapToEntry(entry), nil
}

func (s *FileService) DownloadFile(ctx context.Context, filePath string) (io.Reader, string, error) {
	entry, err := s.GetFile(ctx, filePath)
	if err != nil {
		return nil, "", err
	}

	if entry.IsDir {
		return nil, "", errors.New("cannot download a directory")
	}

	var buf bytes.Buffer

	if err := s.filer.Download(filePath, nil, func(reader io.Reader) error {
		_, err := io.Copy(&buf, reader)
		return err
	}); err != nil {
		return nil, "", err
	}

	return &buf, entry.MimeType, nil
}

func (s *FileService) CreateFile(_ context.Context, content io.Reader, fullName string, fileSize int64) (int64, error) {
	result, err := s.filer.Upload(content, fileSize, fullName, "", "")
	if err != nil {
		return 0, err
	}

	return result.Size, nil
}

func (s *FileService) ListEntries(_ context.Context, dirpath string, limit int, cursor string) ([]file.Entry, string, error) {
	// parse cursor
	cursorObj, _ := decodeCursor(cursor)

	query := url.Values{}
	query.Set("limit", fmt.Sprintf("%d", limit))
	if cursorObj != nil && cursorObj.LastFileName != nil {
		query.Set("lastFileName", *cursorObj.LastFileName)
	}

	header := map[string]string{
		"Accept": "application/json",
	}

	data, code, err := s.filer.Get(dirpath, query, header)
	if err != nil {
		return nil, "", err
	}

	if code != http.StatusOK {
		return nil, "", errors.New("failed to list files and directories")
	}

	var resp listDirectoryEntriesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, "", err
	}

	return resp.mapToEntries(), resp.mapToCursor(), nil
}

type cursor struct {
	LastFileName *string
}

func newCursor(lastFileName string) *cursor {
	return &cursor{LastFileName: &lastFileName}
}

func (c *cursor) encode() string {
	data, _ := json.Marshal(c)
	return base64.StdEncoding.EncodeToString(data)
}

func decodeCursor(cursorStr string) (*cursor, error) {
	data, err := base64.StdEncoding.DecodeString(cursorStr)
	if err != nil {
		return nil, err
	}

	var cursorObj cursor
	if err := json.Unmarshal(data, &cursorObj); err != nil {
		return nil, err
	}

	return &cursorObj, nil
}

type listDirectoryEntriesResponse struct {
	Path                  string
	Entries               []filer.Entry
	Limit                 int
	LastFileName          string
	ShouldDisplayLoadMore bool
	EmptyFolder           bool
}

func (r *listDirectoryEntriesResponse) mapToEntries() []file.Entry {
	var entries []file.Entry
	for _, entry := range r.Entries {
		entries = append(entries, *mapToEntry(entry))
	}

	return entries
}

func (r *listDirectoryEntriesResponse) mapToCursor() string {
	var cursor string
	if r.ShouldDisplayLoadMore {
		cursor = newCursor(r.LastFileName).encode()
	}

	return cursor
}

func mapToEntry(entry filer.Entry) *file.Entry {
	e := file.Entry{
		Name:      entry.FullPath.Name(),
		Size:      entry.FileSize,
		Mode:      entry.Mode,
		MimeType:  entry.Mime,
		MD5:       entry.Md5,
		IsDir:     entry.IsDirectory(),
		CreatedAt: entry.Crtime,
		UpdatedAt: entry.Mtime,
	}

	// remove the root path from the full path
	entryPath := entry.FullPath.Split()
	entryPath[0] = "/"
	e.FullPath = filepath.ToSlash(filepath.Join(entryPath...))

	return &e
}
