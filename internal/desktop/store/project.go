package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *Store) CreateProject(ctx context.Context, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrInvalid
	}
	now := time.Now().UTC()
	r, err := s.db.ExecContext(ctx, "INSERT INTO project(name,created_at) VALUES(?,?)", name, now.Format(time.RFC3339Nano))
	if err != nil {
		return Project{}, err
	}
	id, err := r.LastInsertId()
	return Project{ID: id, Name: name, CreatedAt: now}, err
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,created_at FROM project ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		var p Project
		var created string
		if err := rows.Scan(&p.ID, &p.Name, &created); err != nil {
			return nil, err
		}
		p.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	return changed(s.db.ExecContext(ctx, "DELETE FROM project WHERE id=?", id))
}

func (s *Store) SetProjectVideos(ctx context.Context, projectID int64, ids []int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var id int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM project WHERE id=?", projectID).Scan(&id); err != nil {
			return missing(err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM project_video WHERE project_id=?", projectID); err != nil {
			return err
		}
		for _, id := range ids {
			var visible int64
			if err := tx.QueryRowContext(ctx, "SELECT id FROM video WHERE id=? AND deleted_at IS NULL", id).Scan(&visible); err != nil {
				return missing(err)
			}
			if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO project_video(project_id,video_id) VALUES(?,?)", projectID, id); err != nil {
				return err
			}
		}
		return nil
	})
}
