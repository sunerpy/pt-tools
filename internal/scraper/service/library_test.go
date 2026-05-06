package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

func newLibService(t *testing.T) (*LibraryService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	svc, err := NewLibraryService(LibraryConfig{DB: db})
	require.NoError(t, err)
	return svc, db
}

func TestLibrary_NewLibraryService_NilDB(t *testing.T) {
	_, err := NewLibraryService(LibraryConfig{})
	require.Error(t, err)
}

func TestLibrary_Create_Success(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name:        "Movies",
		Type:        "movie",
		Path:        tmpDir,
		ProviderIDs: []string{"tmdb", "douban"},
	})
	require.NoError(t, err)
	require.Greater(t, lib.ID, uint(0))
	require.Equal(t, "tmdb,douban", lib.ProviderIDs)
	require.Equal(t, "universal", lib.NfoDialect)
	require.True(t, lib.Enabled)
}

func TestLibrary_Create_DefaultType(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name: "NoType",
		Path: tmpDir,
	})
	require.NoError(t, err)
	require.Equal(t, "mixed", lib.Type)
}

func TestLibrary_Create_EmptyName(t *testing.T) {
	svc, _ := newLibService(t)
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Path: "/tmp"})
	require.Error(t, err)
}

func TestLibrary_Create_EmptyPath(t *testing.T) {
	svc, _ := newLibService(t)
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "X"})
	require.Error(t, err)
}

func TestLibrary_Create_InvalidPath(t *testing.T) {
	svc, _ := newLibService(t)
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name: "Bad",
		Path: "/nonexistent/path/xyz/does/not/exist",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestLibrary_Create_PathIsFile(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, createEmptyFile(filePath))
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name: "FileNotDir",
		Path: filePath,
	})
	require.Error(t, err)
}

func TestLibrary_Create_DuplicateName(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "A", Path: tmpDir})
	require.NoError(t, err)
	_, err = svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "A", Path: tmpDir})
	require.Error(t, err)
}

func TestLibrary_Create_InvalidConnectorID(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	invalidID := uint(999)
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name:        "X",
		Path:        tmpDir,
		ConnectorID: &invalidID,
	})
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestLibrary_Create_ValidConnectorID(t *testing.T) {
	svc, db := newLibService(t)
	tmpDir := t.TempDir()
	conn := store.ConnectorConfig{Type: "jellyfin", Name: "jf1", BaseURL: "http://x"}
	require.NoError(t, db.Create(&conn).Error)
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name:        "WithConn",
		Path:        tmpDir,
		ConnectorID: &conn.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, lib.ConnectorID)
	require.Equal(t, conn.ID, *lib.ConnectorID)
}

func TestLibrary_Get_NotFound(t *testing.T) {
	svc, _ := newLibService(t)
	_, err := svc.GetLibrary(context.Background(), 999)
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestLibrary_Get_Success(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "G", Path: tmpDir})
	require.NoError(t, err)
	got, err := svc.GetLibrary(context.Background(), lib.ID)
	require.NoError(t, err)
	require.Equal(t, lib.Name, got.Name)
}

func TestLibrary_Update_PartialFields(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "Orig", Path: tmpDir})
	require.NoError(t, err)

	newName := "Updated"
	newEnabled := false
	updated, err := svc.UpdateLibrary(context.Background(), lib.ID, UpdateLibraryRequest{
		Name:    &newName,
		Enabled: &newEnabled,
	})
	require.NoError(t, err)
	require.Equal(t, "Updated", updated.Name)
	require.False(t, updated.Enabled)
	require.Equal(t, tmpDir, updated.Path)
}

func TestLibrary_Update_ProviderIDs(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{
		Name:        "P",
		Path:        tmpDir,
		ProviderIDs: []string{"tmdb"},
	})
	require.NoError(t, err)

	updated, err := svc.UpdateLibrary(context.Background(), lib.ID, UpdateLibraryRequest{
		ProviderIDs: []string{"tmdb", "douban", "llm"},
	})
	require.NoError(t, err)
	require.Equal(t, "tmdb,douban,llm", updated.ProviderIDs)
}

func TestLibrary_Update_InvalidPath(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "U", Path: tmpDir})
	require.NoError(t, err)

	badPath := "/nonexistent/bad/path"
	_, err = svc.UpdateLibrary(context.Background(), lib.ID, UpdateLibraryRequest{Path: &badPath})
	require.Error(t, err)
}

func TestLibrary_Update_ValidPath(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "U2", Path: tmpDir})
	require.NoError(t, err)

	newDir := t.TempDir()
	updated, err := svc.UpdateLibrary(context.Background(), lib.ID, UpdateLibraryRequest{Path: &newDir})
	require.NoError(t, err)
	require.Equal(t, newDir, updated.Path)
}

func TestLibrary_Update_NotFound(t *testing.T) {
	svc, _ := newLibService(t)
	_, err := svc.UpdateLibrary(context.Background(), 999, UpdateLibraryRequest{})
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestLibrary_Update_InvalidConnector(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "UC", Path: tmpDir})
	require.NoError(t, err)

	bad := uint(999)
	_, err = svc.UpdateLibrary(context.Background(), lib.ID, UpdateLibraryRequest{ConnectorID: &bad})
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestLibrary_Delete_CascadesTasksAndResults(t *testing.T) {
	svc, db := newLibService(t)
	tmpDir := t.TempDir()
	lib, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "X", Path: tmpDir})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&store.ScrapeTask{
			LibraryID:  &lib.ID,
			TaskType:   "movie",
			MediaPath:  fmt.Sprintf("/p%d", i),
			State:      "pending",
			MaxRetries: 3,
		}).Error)
	}
	require.NoError(t, db.Create(&store.ScrapeResult{
		TaskID:    1,
		LibraryID: &lib.ID,
		MediaType: "movie",
	}).Error)

	require.NoError(t, svc.DeleteLibrary(context.Background(), lib.ID))

	var count int64
	require.NoError(t, db.Model(&store.MediaLibraryConfig{}).Where("id = ?", lib.ID).Count(&count).Error)
	require.Equal(t, int64(0), count)

	var taskCount int64
	require.NoError(t, db.Model(&store.ScrapeTask{}).Where("library_id = ?", lib.ID).Count(&taskCount).Error)
	require.Equal(t, int64(0), taskCount)

	var resultCount int64
	require.NoError(t, db.Model(&store.ScrapeResult{}).Where("library_id = ?", lib.ID).Count(&resultCount).Error)
	require.Equal(t, int64(0), resultCount)

	var softDeleted int64
	require.NoError(t, db.Unscoped().Model(&store.MediaLibraryConfig{}).Where("id = ?", lib.ID).Count(&softDeleted).Error)
	require.Equal(t, int64(1), softDeleted)
}

func TestLibrary_Delete_NotFound(t *testing.T) {
	svc, _ := newLibService(t)
	err := svc.DeleteLibrary(context.Background(), 999)
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestLibrary_List_SortedByID(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	_, err := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "B", Path: tmpDir})
	require.NoError(t, err)
	_, err = svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "A", Path: tmpDir})
	require.NoError(t, err)

	libs, err := svc.ListLibraries(context.Background())
	require.NoError(t, err)
	require.Len(t, libs, 2)
	require.Less(t, libs[0].ID, libs[1].ID)
}

func TestLibrary_List_ExcludesSoftDeleted(t *testing.T) {
	svc, _ := newLibService(t)
	tmpDir := t.TempDir()
	lib1, _ := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "Keep", Path: tmpDir})
	lib2, _ := svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "Delete", Path: tmpDir})
	require.NoError(t, svc.DeleteLibrary(context.Background(), lib2.ID))

	libs, err := svc.ListLibraries(context.Background())
	require.NoError(t, err)
	require.Len(t, libs, 1)
	require.Equal(t, lib1.ID, libs[0].ID)
}

func TestLibrary_CustomPathValidator(t *testing.T) {
	db := openTestDB(t)
	customErr := errors.New("custom validator rejected")
	svc, err := NewLibraryService(LibraryConfig{
		DB: db,
		PathValidator: func(path string) error {
			if path == "/allowed" {
				return nil
			}
			return customErr
		},
	})
	require.NoError(t, err)

	_, err = svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "OK", Path: "/allowed"})
	require.NoError(t, err)

	_, err = svc.CreateLibrary(context.Background(), CreateLibraryRequest{Name: "NO", Path: "/denied"})
	require.ErrorIs(t, err, customErr)
}

func createEmptyFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
