package workers

import (
	"github.com/photoprism/photoprism/internal/entity"
	"testing"

	"github.com/photoprism/photoprism/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSync_download(t *testing.T) {
	conf := config.TestConfig()

	t.Logf("database-dsn: %s", conf.DatabaseDsn())

	worker := NewSync(conf)

	assert.IsType(t, &Sync{}, worker)
	account := entity.AccountFixtureWebdavDummy

	complete, err := worker.download(account)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, complete)
}

func TestSync_downloadPath(t *testing.T) {
	conf := config.TestConfig()

	worker := NewSync(conf)

	assert.IsType(t, &Sync{}, worker)
	assert.Equal(t, "/go/src/github.com/photoprism/photoprism/storage/testdata/temp/sync", worker.downloadPath())
}

func TestSync_relatedDownloads(t *testing.T) {
	conf := config.TestConfig()

	worker := NewSync(conf)
	account := entity.AccountFixtureWebdavDummy

	assert.IsType(t, &Sync{}, worker)
	result, err := worker.relatedDownloads(account)
	if err != nil {
		t.Fatal(err)
	}

	assert.IsType(t, Downloads{}, result)
}