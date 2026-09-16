package backend

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestPodFetchDB(t *testing.T) string {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "podcast.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite test db: %v", err)
	}
	defer db.Close()

	schema := `
		CREATE TABLE podcasts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			directory TEXT,
			rssfeed TEXT,
			image_url TEXT,
			summary TEXT,
			author TEXT,
			keywords TEXT,
			explicit INTEGER,
			created_at TEXT,
			last_build_date TEXT
		);
		CREATE TABLE podcast_episodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			podcast_id INTEGER,
			episode_id TEXT,
			name TEXT,
			url TEXT,
			date_of_recording TEXT,
			image_url TEXT,
			total_time INTEGER,
			local_url TEXT,
			local_image_url TEXT,
			description TEXT,
			status TEXT,
			download_time TEXT
		);
		INSERT INTO podcasts (id, name, directory, rssfeed, image_url, summary, author, created_at)
		VALUES (1, 'Show One', 'show_one', 'https://example.com/show1.xml', 'https://example.com/cover.jpg', 'Summary 1', 'Author 1', '2026-08-30 00:00:00');

		INSERT INTO podcast_episodes (id, podcast_id, episode_id, name, url, date_of_recording, total_time, local_url, description, status)
		VALUES 
			(10, 1, 'guid-10', 'Episode 10', 'https://example.com/10.mp3', '2026-08-28 10:00:00', 3600, 'ep10.mp3', 'Desc 10', 'D'),
			(11, 1, 'guid-11', 'Episode 11', 'https://example.com/11.mp3', '2026-08-29 10:00:00', 3600, 'ep11.mp3', 'Desc 11', 'D'),
			(12, 1, 'guid-12', 'Episode 12', 'https://example.com/12.mp3', '2026-08-30 10:00:00', 3600, 'ep12.mp3', 'Desc 12', 'P');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to execute schema: %v", err)
	}

	return dbPath
}

func TestPodFetchDBDirectSync(t *testing.T) {
	t.Parallel()
	dbPath := setupTestPodFetchDB(t)
	be := newPodFetch(Config{DBPath: dbPath})

	ok, err := be.TestConnection(nil)
	if !ok || err != nil {
		t.Fatalf("TestConnection on DB failed: %v", err)
	}

	podcasts, err := be.Podcasts()
	if err != nil || len(podcasts) != 1 {
		t.Fatalf("Podcasts from DB failed: %v, count: %d", err, len(podcasts))
	}
	if podcasts[0].Media.Metadata.Title != "Show One" {
		t.Errorf("expected 'Show One', got %s", podcasts[0].Media.Metadata.Title)
	}
	if len(podcasts[0].Media.Episodes) != 3 {
		t.Errorf("expected 3 episodes, got %d", len(podcasts[0].Media.Episodes))
	}

	pod, err := be.GetPodcast("1")
	if err != nil || pod == nil || pod.ID != "1" {
		t.Fatalf("GetPodcast from DB failed: %v", err)
	}
}

func TestPodFetchModernSchemaCompatibility(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "podcast.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	schema := `
		CREATE TABLE podcasts (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			directory_id TEXT NOT NULL,
			rssfeed TEXT NOT NULL,
			image_url TEXT NOT NULL,
			summary TEXT,
			author TEXT,
			directory_name TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE podcast_episodes (
			id TEXT PRIMARY KEY NOT NULL,
			podcast_id TEXT NOT NULL,
			episode_id TEXT NOT NULL,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			date_of_recording TEXT NOT NULL,
			total_time INTEGER DEFAULT 0 NOT NULL,
			description TEXT DEFAULT '' NOT NULL,
			file_episode_path TEXT
		);
		INSERT INTO podcasts (id, name, directory_id, rssfeed, image_url, summary, author, directory_name)
		VALUES ('uuid-pod-1', 'Modern Show', 'dir-1', 'https://example.com/modern.xml', 'https://example.com/img.jpg', 'Modern summary', 'Modern author', 'podcasts/Modern Show');
		INSERT INTO podcast_episodes (id, podcast_id, episode_id, name, url, date_of_recording, total_time, description, file_episode_path)
		VALUES ('uuid-ep-1', 'uuid-pod-1', 'guid-mod-1', 'Modern Ep 1', 'https://example.com/mod1.mp3', '2026-09-01 12:00:00', 1800, 'Modern Desc', 'podcasts/Modern Show/mod1.mp3');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to initialize modern schema: %v", err)
	}

	be := newPodFetch(Config{DBPath: dbPath})
	podcasts, err := be.Podcasts()
	if err != nil {
		t.Fatalf("be.Podcasts() failed: %v", err)
	}
	if len(podcasts) != 1 {
		t.Fatalf("expected 1 podcast, got %d", len(podcasts))
	}
	if podcasts[0].ID != "uuid-pod-1" {
		t.Errorf("expected ID uuid-pod-1, got %s", podcasts[0].ID)
	}
	if podcasts[0].RelPath != "Modern Show" {
		t.Errorf("expected RelPath Modern Show, got %s", podcasts[0].RelPath)
	}

	single, err := be.GetPodcast("uuid-pod-1")
	if err != nil || single == nil {
		t.Fatalf("GetPodcast failed: %v", err)
	}
	if len(single.Media.Episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(single.Media.Episodes))
	}
}
