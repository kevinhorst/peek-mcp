package state

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	diffBaseFile     = "diff.base"
	diffSnapshotFile = "diff.snapshot"
	planDir          = "plan"
	planLatestFile   = "latest.md"

	draftDiffSuffix = ".draft.diff"
	diffSuffix      = ".diff"
	initialFile     = "000.md"

	dirPerm  = 0o700
	filePerm = 0o600

	MaxSnapshotBytes = 5 * 1024 * 1024
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

type DiffBase struct {
	Sha    string
	Target string
}

type Dir struct {
	root string
}

func NewDir(root string) *Dir {
	return &Dir{root: root}
}

func (d *Dir) pruneAgentDir(path string, cutoff time.Time) {
	sessionDirs, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, sessionDir := range sessionDirs {
		sessionPath := filepath.Join(path, sessionDir.Name())
		if newestModTime(sessionPath).Before(cutoff) {
			os.RemoveAll(sessionPath)
		}
	}
}

func (d *Dir) sessionDir(agent, sessionId string) string {
	return filepath.Join(d.root, sanitize(agent), sanitize(sessionId))
}

func (d *Dir) writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return errors.Wrap(err, "Dir.writeFile: Failed to create state directory")
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), filePerm); err != nil {
		return errors.Wrap(err, "Dir.writeFile: Failed to write temp file")
	}

	if err := os.Rename(tmp, path); err != nil {
		return errors.Wrap(err, "Dir.writeFile: Failed to rename temp file")
	}
	return nil
}

func (d *Dir) Gc(retention time.Duration) {
	if retention <= 0 {
		return
	}

	cutoff := time.Now().Add(-retention)
	agentDirs, err := os.ReadDir(d.root)
	if err != nil {
		return
	}

	for _, agentDir := range agentDirs {
		d.pruneAgentDir(filepath.Join(d.root, agentDir.Name()), cutoff)
	}
}

func (d *Dir) ReadDiffBase(agent, sessionId string) (base DiffBase, ok bool) {
	data, err := os.ReadFile(filepath.Join(d.sessionDir(agent, sessionId), diffBaseFile))
	if err != nil {
		return base, false
	}

	sha, target, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	if !shaPattern.MatchString(sha) {
		return base, false
	}

	base = DiffBase{Sha: sha, Target: target}
	return base, true
}

func (d *Dir) ReadDiffSnapshot(agent, sessionId string) (content string, capturedAt time.Time, ok bool) {
	path := filepath.Join(d.sessionDir(agent, sessionId), diffSnapshotFile)
	info, err := os.Stat(path)
	if err != nil {
		return "", capturedAt, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", capturedAt, false
	}

	return string(data), info.ModTime(), true
}

func (d *Dir) ReadPlanLatest(agent, sessionId string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(d.sessionDir(agent, sessionId), planDir, planLatestFile))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func (d *Dir) ReadPlanVersions(agent, sessionId string) []*PlanVersion {
	dir := filepath.Join(d.sessionDir(agent, sessionId), planDir)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	versions := make([]*PlanVersion, 0)
	for _, file := range files {
		version := planVersionFromFile(dir, file)
		if version != nil {
			versions = append(versions, version)
		}
	}

	sort.Slice(versions, func(i, j int) bool { return versions[i].Index < versions[j].Index })
	return versions
}

func (d *Dir) WriteDiffBase(agent string, base DiffBase, sessionId string) error {
	return d.writeFile(filepath.Join(d.sessionDir(agent, sessionId), diffBaseFile), base.Sha+" "+base.Target)
}

func (d *Dir) WriteDiffSnapshot(agent, content, sessionId string) error {
	if len(content) > MaxSnapshotBytes {
		content = content[:MaxSnapshotBytes] + "\n[peek: snapshot truncated at 5 MB]\n"
	}
	return d.writeFile(filepath.Join(d.sessionDir(agent, sessionId), diffSnapshotFile), content)
}

func (d *Dir) WritePlanLatest(agent, content, sessionId string) error {
	return d.writeFile(filepath.Join(d.sessionDir(agent, sessionId), planDir, planLatestFile), content)
}

func (d *Dir) WritePlanVersion(agent, sessionId string, version *PlanVersion) error {
	name := initialFile
	if version.Index > 0 {
		suffix := draftDiffSuffix
		if version.IsAlteration {
			suffix = diffSuffix
		}
		name = fmt.Sprintf("%03d", version.Index) + suffix
	}
	return d.writeFile(filepath.Join(d.sessionDir(agent, sessionId), planDir, name), version.Content)
}

type PlanVersion struct {
	Content      string
	Index        int
	IsAlteration bool
	ModTime      time.Time
}

func newestModTime(root string) time.Time {
	newest := time.Time{}
	filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

func planVersionFromFile(dir string, file os.DirEntry) *PlanVersion {
	name := file.Name()
	if name == planLatestFile {
		return nil
	}

	info, err := file.Info()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil
	}

	version := &PlanVersion{Content: string(data), ModTime: info.ModTime()}
	if name == initialFile {
		return version
	}

	isAlteration := strings.HasSuffix(name, diffSuffix) && !strings.HasSuffix(name, draftDiffSuffix)
	index, err := strconv.Atoi(strings.SplitN(name, ".", 2)[0])
	if err != nil {
		return nil
	}

	version.Index = index
	version.IsAlteration = isAlteration
	return version
}

func sanitize(component string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	sanitized := replacer.Replace(component)
	if sanitized == "" {
		return "_"
	}
	return sanitized
}
