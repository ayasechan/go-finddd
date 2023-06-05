package finddd

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Matcher interface {
	Match(fsys fs.FS, path string) bool
}
type Option func(Matcher)

type NopMatcher struct{}

func (m *NopMatcher) Match(fsys fs.FS, path string) bool {
	return true
}

type MultiMatcher struct {
	ms []Matcher
}

func (m *MultiMatcher) Add(ms ...Matcher) {
	m.ms = append(m.ms, ms...)
}
func (m *MultiMatcher) Match(fsys fs.FS, path string) bool {
	for _, v := range m.ms {
		if !v.Match(fsys, path) {
			return false
		}
	}
	return true
}
func WithSuffixes(suffixes ...string) Option {
	return func(m Matcher) {
		sm, ok := m.(*SuffixMatcher)
		if ok {
			sm.suffixes = suffixes
		}
	}
}
func NewSuffixMatcher(opts ...Option) *SuffixMatcher {
	sm := &SuffixMatcher{
		suffixes: make([]string, 0),
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

type SuffixMatcher struct {
	suffixes []string
}

func (m *SuffixMatcher) Match(fsys fs.FS, path string) bool {
	if len(m.suffixes) == 0 {
		return true
	}
	name := strings.ToLower(filepath.Base(path))
	for _, v := range m.suffixes {
		if strings.HasSuffix(name, v) {
			return true
		}
	}
	return false
}

func WithFileTypes(ftypes ...FileType) Option {
	return func(m Matcher) {
		ftm, ok := m.(*FiletypeMatcher)
		if ok {
			ftm.ftypes = ftypes
		}
	}
}

func NewFiletypeMatcher(opts ...Option) *FiletypeMatcher {
	ftm := &FiletypeMatcher{ftypes: make([]FileType, 0)}
	for _, opt := range opts {
		opt(ftm)
	}
	return ftm
}

type FileType string

const (
	FT_FILE       FileType = "f"
	FT_DIRECTORY  FileType = "d"
	FT_SYMLINK    FileType = "l"
	FT_EXECUTABLE FileType = "x"
	FT_EMPTY      FileType = "e"
	FT_SOCKET     FileType = "s"
	FT_PIPE       FileType = "p"
)

type FiletypeMatcher struct {
	ftypes []FileType
}

func (m *FiletypeMatcher) Match(fsys fs.FS, path string) bool {
	if len(m.ftypes) == 0 {
		return true
	}
	info, err := fs.Stat(fsys, path)
	if err != nil {
		return false
	}
	mode := info.Mode()
	fns := map[FileType]func() bool{
		FT_FILE:      func() bool { return mode&fs.ModeType == 0 },
		FT_DIRECTORY: func() bool { return mode&fs.ModeDir == 0 },
		FT_SYMLINK:   func() bool { return mode&fs.ModeSymlink == 0 },
		FT_EXECUTABLE: func() bool {
			const S_IXUSR = 0o0100
			const S_IXGRP = 0o0010
			const S_IXOTH = 0o0001
			const xmode = S_IXUSR | S_IXGRP | S_IXOTH
			return mode&xmode > 0
		},
		FT_SOCKET: func() bool { return mode&fs.ModeSocket == 0 },
		FT_PIPE:   func() bool { return mode&fs.ModeNamedPipe == 0 },
	}
	fns[FT_EMPTY] = func() bool {
		if fns[FT_FILE]() {
			return info.Size() == 0
		}
		if fns[FT_DIRECTORY]() {
			des, err := fs.ReadDir(fsys, path)
			if err != nil {
				return false
			}
			return len(des) == 0
		}
		return false
	}
	for _, ft := range m.ftypes {
		if fns[ft]() {
			return true
		}
	}
	return false
}
func WithTimeOlder(t *time.Time) Option {
	return func(m Matcher) {
		ctm, ok := m.(*ChangeTimeMatcher)
		if ok {
			ctm.older = t
		}
	}
}
func WithTimeNewer(t *time.Time) Option {
	return func(m Matcher) {
		ctm, ok := m.(*ChangeTimeMatcher)
		if ok {
			ctm.newer = t
		}
	}
}

func NewChangeTimeMatcher(opts ...Option) *ChangeTimeMatcher {
	ctm := &ChangeTimeMatcher{}
	for _, opt := range opts {
		opt(ctm)
	}
	return ctm
}

type ChangeTimeMatcher struct {
	older *time.Time
	newer *time.Time
}

func (m *ChangeTimeMatcher) Match(fsys fs.FS, path string) bool {
	ok := true
	if m.older == nil && m.newer == nil {
		return ok
	}
	info, err := fs.Stat(fsys, path)
	if err != nil {
		return false
	}
	mtime := info.ModTime()
	if m.older != nil {
		ok = ok && m.older.Before(mtime)
	}
	if m.newer != nil {
		if m.older != nil {
			assert(m.older.Before(*m.newer), "todo")
		}
		ok = ok && m.newer.After(mtime)
	}
	return ok
}
func WithMinSize(min int64) Option {
	return func(m Matcher) {
		sm, ok := m.(*SizeMatcher)
		if ok {
			sm.min = min
		}
	}
}
func WithMaxSize(max int64) Option {
	return func(m Matcher) {
		sm, ok := m.(*SizeMatcher)
		if ok {
			sm.max = max
		}
	}
}
func NewSizeMatcher(opts ...Option) *SizeMatcher {
	sm := &SizeMatcher{
		min: -1,
		max: -1,
	}
	for _, opt := range opts {
		opt(sm)
	}

	return sm
}

type SizeMatcher struct {
	min int64
	max int64
}

func (m *SizeMatcher) Match(fsys fs.FS, path string) bool {
	ok := true
	if m.min < 0 && m.max < 0 {
		return ok
	}
	info, err := fs.Stat(fsys, path)
	if err != nil {
		return false
	}
	size := info.Size()
	if m.min >= 0 {
		ok = ok && size >= m.min
	}
	if m.max >= 0 {
		assert(m.min <= m.max, "todo")
		ok = ok && size <= m.max
	}
	return ok
}

func WithHidden(hidden bool) Option {
	return func(m Matcher) {
		hm, ok := m.(*HiddenMatcher)
		if ok {
			hm.hidden = hidden
		}
	}
}
func NewHiddenMatcher(opts ...Option) *HiddenMatcher {
	hm := &HiddenMatcher{}
	for _, opt := range opts {
		opt(hm)
	}
	return hm
}

type HiddenMatcher struct {
	hidden bool
}

func (m *HiddenMatcher) Match(fsys fs.FS, path string) bool {
	if m.hidden {
		return true
	}
	return strings.HasPrefix(filepath.Base(path), ".")
}

type IgnoreFileMatcher struct{}

func (m *IgnoreFileMatcher) Match(fsys fs.FS, path string) bool {
	return true
}

func WithMaxDepth(max int) Option {
	return func(m Matcher) {
		dm, ok := m.(*DepthMatcher)
		if ok {
			dm.max = max
		}
	}
}
func WithMinDepth(min int) Option {
	return func(m Matcher) {
		dm, ok := m.(*DepthMatcher)
		if ok {
			dm.min = min
		}
	}
}
func WithExactDepth(exact int) Option {
	return func(m Matcher) {
		dm, ok := m.(*DepthMatcher)
		if ok {
			dm.max = exact
			dm.min = exact
		}
	}
}
func NewDepthMatcher(cur string, opts ...Option) *DepthMatcher {
	dm := &DepthMatcher{
		cur: filepath.SplitList(filepath.Clean(cur)),
		min: -1,
		max: -1,
	}
	for _, opt := range opts {
		opt(dm)
	}
	return dm
}

type DepthMatcher struct {
	cur []string
	min int
	max int
}

func (m *DepthMatcher) Match(fsys fs.FS, path string) bool {
	path = filepath.Clean(path)
	depth := len(m.cur) - len(filepath.SplitList(path))
	ok := true
	if m.min >= 0 {
		ok = ok && depth >= m.min
	}
	if m.max >= 0 {
		assert(m.min <= m.max, "todo")
		ok = ok && depth <= m.max
	}
	return ok
}

func WithMaxResult(max int) Option {
	return func(m Matcher) {
		mrm, ok := m.(*MaxResultMatcher)
		if ok {
			mrm.max = max
		}
	}
}

func NewMaxResultMatcher(opts ...Option) *MaxResultMatcher {
	mrm := &MaxResultMatcher{max: -1}
	for _, opt := range opts {
		opt(mrm)
	}
	return mrm
}

type MaxResultMatcher struct {
	count int
	max   int
}

func (m *MaxResultMatcher) Match(fsys fs.FS, path string) bool {
	if m.max < 0 {
		return true
	}
	ok := m.count < m.max
	if ok {
		m.count += 1
	}
	return ok
}

type FilenameMatchMode int

const (
	FMM_EXACT FilenameMatchMode = iota
	FMM_STR
	FMM_GLOB
	FMM_RE
)

type FilenameMatcher struct {
	rawPattern      string
	compiledPattern *regexp.Regexp
	mode            FilenameMatchMode
	ignoreCase      bool
}

func (m *FilenameMatcher) Match(fsys fs.FS, path string) bool {
	panic("todo")
	m.compiledPattern = regexp.MustCompile("foo")
	name := filepath.Base(path)
	if m.ignoreCase {
		name = strings.ToLower(name)
	}
	switch m.mode {
	case FMM_EXACT:
		return m.rawPattern == name
	case FMM_STR:
		return strings.Contains(name, m.rawPattern)
	case FMM_GLOB:
		ok, err := filepath.Match(m.rawPattern, name)
		if err != nil {
			return false
		}
		return ok
	case FMM_RE:
		return len(m.compiledPattern.FindStringSubmatch(name)) > 0
	}
	return true
}

func assert(ok bool, msg any) {
	if !ok {
		panic(msg)
	}
}
