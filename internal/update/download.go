package update

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// DownloadResult holds paths for a verified package download.
type DownloadResult struct {
	PackagePath string
	Version     string
	AssetName   string
}

// DownloadAndVerify fetches the matching asset and SHA256SUMS, verifies the hash,
// and returns the path to the verified package file in dir (usually os.TempDir()).
func (c *Client) DownloadAndVerify(ctx context.Context, rel Release, target Target, dir string) (DownloadResult, error) {
	asset, err := rel.FindAsset(target.AssetName)
	if err != nil {
		return DownloadResult{}, err
	}
	sumAsset, err := rel.ChecksumAsset()
	if err != nil {
		return DownloadResult{}, err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return DownloadResult{}, err
	}

	sumPath := filepath.Join(dir, checksumAssetName)
	sumFile, err := os.Create(sumPath)
	if err != nil {
		return DownloadResult{}, err
	}
	err = c.Download(ctx, sumAsset.BrowserDownloadURL, sumFile)
	closeErr := sumFile.Close()
	if err != nil {
		_ = os.Remove(sumPath)
		return DownloadResult{}, err
	}
	if closeErr != nil {
		_ = os.Remove(sumPath)
		return DownloadResult{}, closeErr
	}

	sumReader, err := os.Open(sumPath)
	if err != nil {
		_ = os.Remove(sumPath)
		return DownloadResult{}, err
	}
	sums, err := ParseSHA256SUMS(sumReader)
	_ = sumReader.Close()
	_ = os.Remove(sumPath)
	if err != nil {
		return DownloadResult{}, err
	}
	want, err := ExpectedHash(sums, asset.Name)
	if err != nil {
		return DownloadResult{}, err
	}

	pkgPath := filepath.Join(dir, asset.Name)
	pkgFile, err := os.OpenFile(pkgPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return DownloadResult{}, err
	}
	err = c.Download(ctx, asset.BrowserDownloadURL, pkgFile)
	closeErr = pkgFile.Close()
	if err != nil {
		_ = os.Remove(pkgPath)
		return DownloadResult{}, err
	}
	if closeErr != nil {
		_ = os.Remove(pkgPath)
		return DownloadResult{}, closeErr
	}

	if err := VerifyFile(pkgPath, want); err != nil {
		_ = os.Remove(pkgPath)
		return DownloadResult{}, err
	}

	// Ensure the package is executable where needed (AppImage / Linux binary).
	switch target.Kind {
	case InstallAppImage, InstallBinary:
		if err := os.Chmod(pkgPath, 0o755); err != nil {
			_ = os.Remove(pkgPath)
			return DownloadResult{}, err
		}
	}

	return DownloadResult{
		PackagePath: pkgPath,
		Version:     strings.TrimPrefix(rel.TagName, "v"),
		AssetName:   asset.Name,
	}, nil
}

// CheckAvailable returns the latest release if it is newer than localVersion.
// ok is false when no update is available (not an error).
func (c *Client) CheckAvailable(ctx context.Context, localVersion string) (rel Release, ok bool, err error) {
	rel, err = c.LatestRelease(ctx)
	if err != nil {
		return Release{}, false, err
	}
	if !IsNewer(localVersion, rel.TagName) {
		return rel, false, nil
	}
	return rel, true, nil
}
