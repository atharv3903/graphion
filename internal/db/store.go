package db

import (
	"database/sql"
	"github.com/atharv3903/graphion/internal/model"
)

type Store struct {
	DB *sql.DB
}

func (s Store) Outgoing(src int64) ([]model.Edge, error) {
	rows, err := s.DB.Query(`
        SELECT dst_node, distance_m, speed_kmph, closed 
        FROM edges 
        WHERE src_node=?
    `, src)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	edges := make([]model.Edge, 0, 8)

	for rows.Next() {
		var dst int64
		var dist, speed int
		var closed bool

		if err := rows.Scan(&dst, &dist, &speed, &closed); err != nil {
			return nil, err
		}
		if closed {
			continue
		}

		edges = append(edges, model.Edge{
			Src:   src,
			Dst:   dst,
			DistM: dist,
			Speed: speed,
		})
	}

	return edges, nil
}

func (s Store) UpdateEdgeSpeed(edgeID int64, speed int) error {
	_, err := s.DB.Exec(`UPDATE edges SET speed_kmph=? WHERE edge_id=?`, speed, edgeID)
	return err
}

func (s Store) UpdateEdgeClosed(edgeID int64, closed bool) error {
	_, err := s.DB.Exec(`UPDATE edges SET closed=? WHERE edge_id=?`, closed, edgeID)
	return err
}
