#!/usr/bin/env python3
"""
opencode SQLite 폴링 PoC
- opencode가 실행 중일 때 외부에서 DB를 read-only로 관찰
- part 테이블의 새 row를 실시간 감지하여 출력
"""

import sqlite3
import json
import time
import sys
import os
from datetime import datetime

DB_PATH = os.environ.get(
    "OPENCODE_DB",
    os.path.expanduser("~/.local/share/opencode/opencode.db"),
)
POLL_INTERVAL = float(os.environ.get("POLL_INTERVAL", "0.5"))


def connect():
    conn = sqlite3.connect(f"file:{DB_PATH}?mode=ro", uri=True)
    conn.execute("PRAGMA journal_mode=wal")
    conn.row_factory = sqlite3.Row
    return conn


def format_part(row):
    data = json.loads(row["data"]) if row["data"] else {}
    ptype = data.get("type", "?")
    ts = datetime.fromtimestamp(row["time_created"] / 1000).strftime("%H:%M:%S")

    if ptype == "text":
        text = data.get("text", "")
        preview = text[:120].replace("\n", " ")
        return f"[{ts}] 💬 text: {preview}"
    elif ptype == "tool":
        tool_name = data.get("tool", data.get("name", "?"))
        state = data.get("state", "")
        return f"[{ts}] 🔧 tool: {tool_name} ({state})"
    elif ptype == "patch":
        path = data.get("path", "?")
        return f"[{ts}] 📝 patch: {path}"
    elif ptype == "reasoning":
        text = data.get("text", "")
        preview = text[:80].replace("\n", " ")
        return f"[{ts}] 🧠 reasoning: {preview}"
    elif ptype == "step-start":
        return f"[{ts}] ▶ step-start"
    elif ptype == "step-finish":
        finish = data.get("finish_reason", "")
        return f"[{ts}] ■ step-finish ({finish})"
    elif ptype == "compaction":
        return f"[{ts}] 📦 compaction"
    else:
        return f"[{ts}] ❓ {ptype}"


def get_latest_timestamp(conn):
    cur = conn.execute("SELECT MAX(time_created) FROM part")
    row = cur.fetchone()
    return row[0] if row[0] else 0


def poll(conn, since):
    cur = conn.execute(
        """
        SELECT p.id, p.session_id, p.time_created, p.data, s.title
        FROM part p
        LEFT JOIN session s ON p.session_id = s.id
        WHERE p.time_created > ?
        ORDER BY p.time_created ASC
        """,
        (since,),
    )
    return cur.fetchall()


def main():
    if not os.path.exists(DB_PATH):
        print(f"DB not found: {DB_PATH}")
        sys.exit(1)

    conn = connect()

    # Start from now (skip history)
    if "--from-start" in sys.argv:
        since = 0
    else:
        since = get_latest_timestamp(conn)

    print(f"Polling {DB_PATH}")
    print(f"Interval: {POLL_INTERVAL}s | Since: {since}")
    print(f"Waiting for new parts...\n")

    try:
        while True:
            rows = poll(conn, since)
            for row in rows:
                session_title = row["title"] or row["session_id"][:12]
                line = format_part(row)
                print(f"  [{session_title}] {line}")
                since = row["time_created"]

            if rows:
                sys.stdout.flush()

            time.sleep(POLL_INTERVAL)
    except KeyboardInterrupt:
        print("\nStopped.")
    finally:
        conn.close()


if __name__ == "__main__":
    main()
