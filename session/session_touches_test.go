package session

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSession_AddFileTouch(t *testing.T) {
	// read-and-write-counts
	t.Run("read-and-write-counts", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddFileTouch(&FileTouch{Path: "/a.go"})
		s.AddFileTouch(&FileTouch{Path: "/a.go", Write: true})
		s.AddFileTouch(&FileTouch{Path: "/a.go", Write: true})

		assert.Equal(t, 1, s.TouchedFiles["/a.go"].Reads)
		assert.Equal(t, 2, s.TouchedFiles["/a.go"].Writes)
	})

	// cap-drops-new-paths
	t.Run("cap-drops-new-paths", func(t *testing.T) {
		s := provideCompleteSession()
		for i := 0; i < maxTouchedFiles; i++ {
			s.AddFileTouch(&FileTouch{Path: "/f" + strconv.Itoa(i)})
		}
		s.AddFileTouch(&FileTouch{Path: "/overflow"})
		s.AddFileTouch(&FileTouch{Path: "/f0"})

		assert.Len(t, s.TouchedFiles, maxTouchedFiles)
		assert.NotContains(t, s.TouchedFiles, "/overflow")
		assert.Equal(t, 2, s.TouchedFiles["/f0"].Reads)
	})
}
