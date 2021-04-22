package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/photoprism/photoprism/internal/service"

	"github.com/photoprism/photoprism/internal/photoprism"

	"github.com/photoprism/photoprism/internal/entity"
	"github.com/photoprism/photoprism/pkg/fs"
	"github.com/photoprism/photoprism/pkg/txt"

	"github.com/photoprism/photoprism/internal/config"
	"github.com/urfave/cli"
)

// RestoreCommand configures the backup cli command.
var RestoreCommand = cli.Command{
	Name:      "restore",
	Usage:     "Restores album and index backups",
	UsageText: `A custom index sql backup FILENAME may be passed as first argument. By default, the backup path is searched.`,
	Flags:     restoreFlags,
	Action:    restoreAction,
}

var restoreFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "force, f",
		Usage: "overwrite existing index",
	},
	cli.BoolFlag{
		Name:  "albums, a",
		Usage: "restore album yaml file backups",
	},
	cli.StringFlag{
		Name:  "albums-path",
		Usage: "custom album yaml file backup `PATH`",
	},
	cli.BoolFlag{
		Name:  "index, i",
		Usage: "restore index sql database backup",
	},
	cli.StringFlag{
		Name:  "index-path",
		Usage: "custom index sql database backup `PATH`",
	},
}

// restoreAction restores a database backup.
func restoreAction(ctx *cli.Context) error {
	// Use command argument as backup file name.
	indexFileName := ctx.Args().First()
	indexPath := ctx.String("index-path")
	restoreIndex := ctx.Bool("index") || indexFileName != "" || indexPath != ""

	albumsPath := ctx.String("albums-path")
	restoreAlbums := ctx.Bool("albums") || albumsPath != ""

	if !restoreIndex && !restoreAlbums {
		for _, flag := range restoreFlags {
			fmt.Println(flag.String())
		}

		return nil
	}

	start := time.Now()

	conf := config.NewConfig(ctx)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := conf.Init(); err != nil {
		return err
	}

	if restoreIndex {
		// If empty, use default backup file name.
		if indexFileName == "" {
			if indexPath == "" {
				indexPath = filepath.Join(conf.BackupPath(), conf.DatabaseDriver())
			}

			matches, err := filepath.Glob(filepath.Join(regexp.QuoteMeta(indexPath), "*.sql"))

			if err != nil {
				return err
			}

			if len(matches) == 0 {
				log.Errorf("no backup files found in %s", indexPath)
				return nil
			}

			indexFileName = matches[len(matches)-1]
		}

		if !fs.FileExists(indexFileName) {
			log.Errorf("backup file not found: %s", indexFileName)
			return nil
		}

		counts := struct{ Photos int }{}

		conf.Db().Unscoped().Table("photos").
			Select("COUNT(*) AS photos").
			Take(&counts)

		if counts.Photos == 0 {
			// Do nothing;
		} else if !ctx.Bool("force") {
			return fmt.Errorf("use --force to replace exisisting index with %d photos", counts.Photos)
		} else {
			log.Warnf("replacing existing index with %d photos", counts.Photos)
		}

		log.Infof("restoring index from %s", txt.Quote(indexFileName))

		sqlBackup, err := ioutil.ReadFile(indexFileName)

		if err != nil {
			return err
		}

		entity.SetDbProvider(conf)
		tables := entity.Entities

		var cmd *exec.Cmd

		switch conf.DatabaseDriver() {
		case config.MySQL, config.MariaDB:
			cmd = exec.Command(
				conf.MysqlBin(),
				"--protocol", "tcp",
				"-h", conf.DatabaseHost(),
				"-P", conf.DatabasePortString(),
				"-u", conf.DatabaseUser(),
				"-p"+conf.DatabasePassword(),
				"-f",
				conf.DatabaseName(),
			)
		case config.SQLite:
			log.Infoln("dropping existing tables")
			tables.Drop()
			cmd = exec.Command(
				conf.SqliteBin(),
				conf.DatabaseDsn(),
			)
		default:
			return fmt.Errorf("unsupported database type: %s", conf.DatabaseDriver())
		}

		// Fetch command output.
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr

		stdin, err := cmd.StdinPipe()

		if err != nil {
			log.Fatal(err)
		}

		go func() {
			defer stdin.Close()
			if _, err := io.WriteString(stdin, string(sqlBackup)); err != nil {
				log.Errorf(err.Error())
			}
		}()

		// Run backup command.
		if err := cmd.Run(); err != nil {
			if stderr.String() != "" {
				log.Debugln(stderr.String())
				log.Warnf("index could not be restored completely")
			}
		}
	}

	log.Infoln("migrating database")

	conf.InitDb()

	if restoreAlbums {
		service.SetConfig(conf)

		if albumsPath == "" {
			albumsPath = conf.AlbumsPath()
		}

		if !fs.PathExists(albumsPath) {
			log.Warnf("albums path %s not found", txt.Quote(albumsPath))
		} else {
			log.Infof("restoring albums from %s", txt.Quote(albumsPath))

			if count, err := photoprism.RestoreAlbums(albumsPath, true); err != nil {
				return err
			} else {
				log.Infof("%d albums restored from yaml files", count)
			}
		}
	}

	elapsed := time.Since(start)

	log.Infof("backup restored in %s", elapsed)

	conf.Shutdown()

	return nil
}
