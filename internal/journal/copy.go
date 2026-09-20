// Package private copy helpers used by the store.
package journal

import (
	"io"
	"os"
	"path"
)

// copyBetween copies src/<rel> to dst/<rel>. A missing source is reported as
// os.ErrNotExist so the caller can decide whether that matters (it does for an
// export, it does not when there is simply no previous version to keep).
func copyBetween(src *os.Root, rel string, dst *os.Root, dstRel string) error {
	in, err := src.Open(rel)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		// Only regular files are ever copied: a symlink or a fifo inside the
		// workspace must not become a host file.
		return nil
	}
	if err := dst.MkdirAll(path.Dir(dstRel), 0o700); err != nil {
		return err
	}
	out, err := dst.OpenFile(dstRel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		_ = out.Close()
		return err
	}
	_, cerr := io.Copy(out, in)
	if serr := out.Close(); cerr == nil {
		cerr = serr
	}
	return cerr
}

// copyWithin copies src/<rel> to dst/<dstRel> with an explicit mode. The mode
// is applied with Chmod because the process umask would otherwise strip the
// execute bit that tells the user a file needs review.
func copyWithin(src *os.Root, rel string, dst *os.Root, dstRel string, mode os.FileMode) error {
	in, err := src.Open(rel)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := dst.MkdirAll(path.Dir(dstRel), 0o700); err != nil {
		return err
	}
	out, err := dst.OpenFile(dstRel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if err := out.Chmod(mode); err != nil {
		_ = out.Close()
		return err
	}
	_, cerr := io.Copy(out, in)
	if serr := out.Close(); cerr == nil {
		cerr = serr
	}
	return cerr
}
