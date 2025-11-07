CREATE DATABASE IF NOT EXISTS routing;
USE routing;

DROP TABLE IF EXISTS edges;
DROP TABLE IF EXISTS nodes;

CREATE TABLE nodes (
  node_id   BIGINT PRIMARY KEY,
  lat       DOUBLE NOT NULL,
  lon       DOUBLE NOT NULL
) ENGINE=InnoDB;

CREATE TABLE edges (
  edge_id     BIGINT AUTO_INCREMENT PRIMARY KEY,
  src_node    BIGINT NOT NULL,
  dst_node    BIGINT NOT NULL,
  distance_m  INT    NOT NULL,
  speed_kmph  INT    NOT NULL,
  closed      TINYINT(1) NOT NULL DEFAULT 0,
  INDEX ix_src (src_node),
  INDEX ix_dst (dst_node),
  CONSTRAINT fk_src FOREIGN KEY (src_node) REFERENCES nodes(node_id),
  CONSTRAINT fk_dst FOREIGN KEY (dst_node) REFERENCES nodes(node_id)
) ENGINE=InnoDB;
