package backend

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var (
	podfetchDBPoolMu syncRWMutex
	podfetchDBPool   = make(map[string]*sql.DB)
)

func getPodfetchDB(dbPath string) (*sql.DB, error) {
	podfetchDBPoolMu.Lock()
	defer podfetchDBPoolMu.Unlock()
	if db, ok := podfetchDBPool[dbPath]; ok {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		_ = db.Close()
		delete(podfetchDBPool, dbPath)
	}
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	podfetchDBPool[dbPath] = db
	return db, nil
}

func podfetchHasColumn(db *sql.DB, table, column string) bool {
	if table != "podcasts" && table != "podcast_episodes" && table != "podcast_settings" {
		return false
	}
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err == nil {
			if strings.EqualFold(name, column) {
				return true
			}
		}
	}
	return false
}

func podfetchPodcastsDirCol(db *sql.DB) string {
	if podfetchHasColumn(db, "podcasts", "directory_name") {
		return "directory_name"
	}
	return "directory"
}

func podfetchEpisodeFileCol(db *sql.DB) string {
	if podfetchHasColumn(db, "podcast_episodes", "file_episode_path") {
		return "file_episode_path"
	}
	return "local_url"
}

func podfetchColOrEmpty(db *sql.DB, table, col string) string {
	if podfetchHasColumn(db, table, col) {
		return col
	}
	return "'' AS " + col
}

func fetchPodFetchPodcastsDB(dbPath string) ([]Podcast, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("dbPath is empty")
	}
	if fi, err := os.Stat(dbPath); err != nil || fi.IsDir() {
		return nil, fmt.Errorf("podfetch db file does not exist: %s", dbPath)
	}

	db, err := getPodfetchDB(dbPath)
	if err != nil {
		return nil, err
	}

	dirCol := podfetchPodcastsDirCol(db)
	imgCol := podfetchColOrEmpty(db, "podcasts", "image_url")
	sumCol := podfetchColOrEmpty(db, "podcasts", "summary")
	autCol := podfetchColOrEmpty(db, "podcasts", "author")
	query := fmt.Sprintf("SELECT id, name, %s, rssfeed, %s, %s, %s FROM podcasts ORDER BY id ASC", dirCol, imgCol, sumCol, autCol)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var podcasts []Podcast
	for rows.Next() {
		var idVal interface{}
		var name, directory, rssfeed, imageURL, summary, author sql.NullString
		if err := rows.Scan(&idVal, &name, &directory, &rssfeed, &imageURL, &summary, &author); err != nil {
			continue
		}

		idStr := fmt.Sprintf("%v", idVal)
		dir := strings.TrimPrefix(directory.String, "podcasts/")
		if dir == "" {
			dir = sanitizePodcastName(name.String)
		}

		eps, _ := fetchPodFetchEpisodesForPodcastDB(db, idStr)

		pod := Podcast{
			ID:        idStr,
			RelPath:   dir,
			MediaType: "podcast",
			Media: PodcastMedia{
				ID: idStr,
				Metadata: PodcastMetadata{
					Title:       name.String,
					Author:      author.String,
					Description: summary.String,
					FeedURL:     rssfeed.String,
					ImageURL:    imageURL.String,
				},
				Episodes: eps,
			},
		}
		podcasts = append(podcasts, pod)
	}

	if err := rows.Err(); err != nil {
		return podcasts, fmt.Errorf("error reading podcast rows: %w", err)
	}

	return podcasts, nil
}

func fetchPodFetchEpisodesForPodcastDB(db *sql.DB, podcastID string) ([]Episode, error) {
	fileCol := podfetchEpisodeFileCol(db)
	hasStatus := podfetchHasColumn(db, "podcast_episodes", "status")
	hasDesc := podfetchHasColumn(db, "podcast_episodes", "description")

	statusExpr := "'D'"
	if hasStatus {
		statusExpr = "status"
	}
	descExpr := "''"
	if hasDesc {
		descExpr = "description"
	}

	query := fmt.Sprintf("SELECT id, episode_id, name, url, date_of_recording, total_time, %s, %s, %s FROM podcast_episodes WHERE podcast_id = ? ORDER BY id DESC", fileCol, descExpr, statusExpr)
	rows, err := db.Query(query, podcastID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		var idVal interface{}
		var epID, name, url, dateOfRec, localURL, description, status sql.NullString
		var totalTime sql.NullFloat64

		if err := rows.Scan(&idVal, &epID, &name, &url, &dateOfRec, &totalTime, &localURL, &description, &status); err != nil {
			continue
		}

		idStr := fmt.Sprintf("%v", idVal)
		guid := epID.String
		if guid == "" {
			guid = idStr
		}

		dateStr := dateOfRec.String
		dur := totalTime.Float64

		ep := Episode{
			ID:           idStr,
			Title:        name.String,
			GUID:         guid,
			PubDate:      dateStr,
			PublishedAt:  ParsePubDate(dateStr),
			Duration:     dur,
			Description:  description.String,
			EnclosureURL: url.String,
		}

		cleanPath := strings.TrimPrefix(localURL.String, "podcasts/")
		if cleanPath != "" {
			ep.AudioFile = &PodcastAudioFile{
				Duration: dur,
				Metadata: &AudioFileMetadata{
					Filename: filepath.Base(cleanPath),
					Path:     cleanPath,
					RelPath:  cleanPath,
				},
			}
		}

		episodes = append(episodes, ep)
	}

	if err := rows.Err(); err != nil {
		return episodes, fmt.Errorf("error reading episode rows: %w", err)
	}

	return episodes, nil
}

func fetchPodFetchPodcastDB(dbPath, id string) (*Podcast, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("dbPath is empty")
	}
	db, err := getPodfetchDB(dbPath)
	if err != nil {
		return nil, err
	}

	dirCol := podfetchPodcastsDirCol(db)
	imgCol := podfetchColOrEmpty(db, "podcasts", "image_url")
	sumCol := podfetchColOrEmpty(db, "podcasts", "summary")
	autCol := podfetchColOrEmpty(db, "podcasts", "author")
	var idVal interface{}
	var name, directory, rssfeed, imageURL, summary, author sql.NullString

	query := fmt.Sprintf("SELECT id, name, %s, rssfeed, %s, %s, %s FROM podcasts WHERE id = ? OR name = ? OR %s = ? LIMIT 1", dirCol, imgCol, sumCol, autCol, dirCol)
	err = db.QueryRow(query, id, id, id).Scan(&idVal, &name, &directory, &rssfeed, &imageURL, &summary, &author)
	if err != nil {
		return nil, err
	}

	idStr := fmt.Sprintf("%v", idVal)
	dir := strings.TrimPrefix(directory.String, "podcasts/")
	if dir == "" {
		dir = sanitizePodcastName(name.String)
	}

	eps, _ := fetchPodFetchEpisodesForPodcastDB(db, idStr)

	pod := &Podcast{
		ID:        idStr,
		RelPath:   dir,
		MediaType: "podcast",
		Media: PodcastMedia{
			ID: idStr,
			Metadata: PodcastMetadata{
				Title:       name.String,
				Author:      author.String,
				Description: summary.String,
				FeedURL:     rssfeed.String,
				ImageURL:    imageURL.String,
			},
			Episodes: eps,
		},
	}
	return pod, nil
}
