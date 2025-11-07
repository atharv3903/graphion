import osmium
import argparse
import mysql.connector
import math

# haversine distance for road segment length (meters)
def haversine(lat1, lon1, lat2, lon2):
    R = 6371000
    phi1 = math.radians(lat1)
    phi2 = math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlambda = math.radians(lon2 - lon1)

    a = math.sin(dphi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(dlambda/2)**2
    return int(R * 2 * math.atan2(math.sqrt(a), math.sqrt(1-a)))

def speed_from_tags(tags):
    if "maxspeed" in tags:
        try:
            return int("".join(c for c in tags["maxspeed"] if c.isdigit()))
        except:
            pass
    hw = tags.get("highway", "")
    if hw in ("motorway", "trunk"): return 90
    if hw in ("primary", "secondary", "tertiary"): return 60
    return 40


class OSMHandler(osmium.SimpleHandler):
    def __init__(self):
        super().__init__()
        self.nodes = {}  # id -> (lat, lon)
        self.edges = []  # (src, dst, dist_m, speed)

    def node(self, n):
        self.nodes[n.id] = (n.location.lat, n.location.lon)

    def way(self, w):
        if "highway" not in w.tags:
            return

        refs = [nd.ref for nd in w.nodes]
        if len(refs) < 2:
            return

        speed = speed_from_tags(dict(w.tags))

        for i in range(len(refs)-1):
            a = refs[i]
            b = refs[i+1]
            if a in self.nodes and b in self.nodes:
                lat1, lon1 = self.nodes[a]
                lat2, lon2 = self.nodes[b]
                dist = haversine(lat1, lon1, lat2, lon2)

                # forward edge
                self.edges.append((a, b, dist, speed))

                # backward for two-way roads
                if w.tags.get("oneway", "no") == "no":
                    self.edges.append((b, a, dist, speed))


def import_to_mysql(pbf_path, host, user, password, dbname):
    print("Parsing PBF with pyosmium…")
    handler = OSMHandler()
    handler.apply_file(pbf_path)

    print(f"Nodes: {len(handler.nodes)}")
    print(f"Edges: {len(handler.edges)}")

    conn = mysql.connector.connect(
        host=host, user=user, password=password, database=dbname
    )
    cur = conn.cursor()

    print("Clearing old data…")
    cur.execute("DELETE FROM edges")
    cur.execute("DELETE FROM nodes")
    conn.commit()

    print("Inserting nodes…")
    nsql = "INSERT INTO nodes (node_id, lat, lon) VALUES (%s, %s, %s)"
    batch = []
    for nid, (lat, lon) in handler.nodes.items():
        batch.append((nid, lat, lon))
        if len(batch) >= 5000:
            cur.executemany(nsql, batch)
            conn.commit()
            batch.clear()
    if batch:
        cur.executemany(nsql, batch)
        conn.commit()

    print("Inserting edges…")
    esql = """INSERT INTO edges
              (src_node, dst_node, distance_m, speed_kmph, closed)
              VALUES (%s, %s, %s, %s, 0)"""
    batch = []
    for e in handler.edges:
        batch.append(e)
        if len(batch) >= 5000:
            cur.executemany(esql, batch)
            conn.commit()
            batch.clear()
    if batch:
        cur.executemany(esql, batch)
        conn.commit()

    print("✅ Import finished")
    cur.close()
    conn.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--pbf", required=True)
    parser.add_argument("--dsn_host", default="127.0.0.1")
    parser.add_argument("--dsn_user", default="root")
    parser.add_argument("--dsn_pass", default="")
    parser.add_argument("--dsn_db", default="routing")
    args = parser.parse_args()

    import_to_mysql(args.pbf, args.dsn_host, args.dsn_user, args.dsn_pass, args.dsn_db)
