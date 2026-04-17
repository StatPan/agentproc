#!/usr/bin/env python3
"""
aproc WebSocket server
- SQLite 폴링 → WebSocket 브로드캐스트
- 브라우저에서 실시간 로그 수신
"""

import asyncio
import json
import os
import sqlite3
from contextlib import asynccontextmanager
from datetime import datetime

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from fastapi.staticfiles import StaticFiles

DB_PATH = os.environ.get(
    "OPENCODE_DB",
    os.path.expanduser("~/.local/share/opencode/opencode.db"),
)
POLL_INTERVAL = float(os.environ.get("POLL_INTERVAL", "0.5"))


# --- DB ---

def db_connect():
    conn = sqlite3.connect(f"file:{DB_PATH}?mode=ro", uri=True)
    conn.execute("PRAGMA journal_mode=wal")
    conn.row_factory = sqlite3.Row
    return conn


def get_latest_timestamp(conn):
    cur = conn.execute("SELECT MAX(time_created) FROM part")
    row = cur.fetchone()
    return row[0] if row[0] else 0


def poll_new_parts(conn, since):
    cur = conn.execute(
        """
        SELECT p.id, p.session_id, p.time_created, p.data, s.title, s.slug
        FROM part p
        LEFT JOIN session s ON p.session_id = s.id
        WHERE p.time_created > ?
        ORDER BY p.time_created ASC
        LIMIT 200
        """,
        (since,),
    )
    return cur.fetchall()


def get_sessions(conn):
    cur = conn.execute(
        """
        SELECT s.id, s.title, s.slug, s.directory, s.time_created, s.time_updated,
               COUNT(p.id) as part_count,
               MAX(p.time_created) as last_activity
        FROM session s
        LEFT JOIN part p ON p.session_id = s.id
        GROUP BY s.id
        ORDER BY last_activity DESC
        LIMIT 50
        """
    )
    return [dict(r) for r in cur.fetchall()]


def format_part(row):
    data = json.loads(row["data"]) if row["data"] else {}
    ptype = data.get("type", "unknown")
    ts = row["time_created"]

    result = {
        "id": row["id"],
        "session_id": row["session_id"],
        "session_title": row["title"] or row["slug"] or row["session_id"][:12],
        "type": ptype,
        "timestamp": ts,
        "time_str": datetime.fromtimestamp(ts / 1000).strftime("%H:%M:%S"),
    }

    if ptype == "text":
        result["text"] = data.get("text", "")[:500]
    elif ptype == "tool":
        result["tool_name"] = data.get("tool", data.get("name", "?"))
        result["state"] = data.get("state", "")
        input_data = data.get("input", {})
        if isinstance(input_data, dict):
            result["tool_description"] = input_data.get("description", "")
    elif ptype == "patch":
        result["path"] = data.get("path", "?")
    elif ptype == "reasoning":
        result["text"] = data.get("text", "")[:300]
    elif ptype == "step-finish":
        result["finish_reason"] = data.get("finish_reason", "")

    return result


# --- WebSocket manager ---

class ConnectionManager:
    def __init__(self):
        self.connections: list[WebSocket] = []

    async def connect(self, ws: WebSocket):
        await ws.accept()
        self.connections.append(ws)

    def disconnect(self, ws: WebSocket):
        self.connections.remove(ws)

    async def broadcast(self, data: dict):
        dead = []
        for ws in self.connections:
            try:
                await ws.send_json(data)
            except Exception:
                dead.append(ws)
        for ws in dead:
            self.connections.remove(ws)


manager = ConnectionManager()


# --- Polling loop ---

async def poll_loop():
    conn = db_connect()
    since = get_latest_timestamp(conn)

    while True:
        try:
            rows = poll_new_parts(conn, since)
            for row in rows:
                part = format_part(row)
                await manager.broadcast({"type": "part", "data": part})
                since = row["time_created"]
        except sqlite3.OperationalError:
            # DB locked momentarily, retry next cycle
            pass
        await asyncio.sleep(POLL_INTERVAL)


# --- App ---

@asynccontextmanager
async def lifespan(app: FastAPI):
    task = asyncio.create_task(poll_loop())
    yield
    task.cancel()


app = FastAPI(lifespan=lifespan)


@app.get("/api/sessions")
def api_sessions():
    conn = db_connect()
    sessions = get_sessions(conn)
    conn.close()
    return sessions


@app.get("/api/sessions/{session_id}/parts")
def api_session_parts(session_id: str, limit: int = 100):
    conn = db_connect()
    cur = conn.execute(
        """
        SELECT p.id, p.session_id, p.time_created, p.data, s.title, s.slug
        FROM part p
        LEFT JOIN session s ON p.session_id = s.id
        WHERE p.session_id = ?
        ORDER BY p.time_created DESC
        LIMIT ?
        """,
        (session_id, limit),
    )
    parts = [format_part(row) for row in cur.fetchall()]
    conn.close()
    parts.reverse()
    return parts


@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket):
    await manager.connect(ws)
    try:
        while True:
            # keep alive, listen for client messages if needed
            await ws.receive_text()
    except WebSocketDisconnect:
        manager.disconnect(ws)


@app.get("/")
def index():
    return HTMLResponse(DASHBOARD_HTML)


# --- Inline dashboard ---

DASHBOARD_HTML = """\
<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>aproc dashboard</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, 'Segoe UI', sans-serif; background: #0d1117; color: #c9d1d9; }

  /* --- Header --- */
  .header {
    padding: 12px 24px; border-bottom: 1px solid #21262d;
    display: flex; align-items: center; gap: 12px;
  }
  .header h1 { font-size: 15px; font-weight: 600; letter-spacing: -0.3px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: #3fb950; }
  .header .status { font-size: 11px; color: #3fb950; }
  .view-toggle { margin-left: auto; display: flex; gap: 4px; }
  .view-toggle button {
    background: #161b22; border: 1px solid #30363d; color: #8b949e;
    padding: 4px 10px; font-size: 11px; cursor: pointer; border-radius: 6px;
  }
  .view-toggle button.active { background: #1f6feb; color: #fff; border-color: #1f6feb; }

  /* --- Board view --- */
  .board {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: 16px; padding: 20px; overflow-y: auto;
    height: calc(100vh - 49px);
  }

  .card {
    background: #161b22; border: 1px solid #21262d; border-radius: 12px;
    display: flex; flex-direction: column; max-height: 420px;
    transition: border-color 0.2s, box-shadow 0.2s;
  }
  .card:hover { border-color: #30363d; box-shadow: 0 4px 12px rgba(0,0,0,0.3); }
  .card.live { border-color: #1f6feb; }
  .card.live .card-status { color: #3fb950; }

  .card-header {
    padding: 14px 16px 10px; border-bottom: 1px solid #21262d;
    display: flex; align-items: flex-start; gap: 10px;
  }
  .card-indicator {
    width: 10px; height: 10px; border-radius: 50%; margin-top: 3px; flex-shrink: 0;
    background: #484f58;
  }
  .card.live .card-indicator { background: #3fb950; animation: pulse 2s infinite; }
  @keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.4; } }

  .card-info { flex: 1; min-width: 0; }
  .card-title {
    font-size: 13px; font-weight: 600; color: #e6edf3;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .card-meta { font-size: 11px; color: #484f58; margin-top: 2px; }
  .card-status { font-size: 11px; font-weight: 500; margin-top: 2px; }

  .card-logs {
    flex: 1; overflow-y: auto; padding: 8px 0; font-family: 'SF Mono', monospace;
  }
  .card-logs::-webkit-scrollbar { width: 4px; }
  .card-logs::-webkit-scrollbar-thumb { background: #30363d; border-radius: 2px; }

  .log {
    padding: 2px 14px; font-size: 11px; line-height: 1.7;
    display: flex; gap: 6px; align-items: baseline;
  }
  .log:hover { background: #1c2129; }
  .log-time { color: #484f58; flex-shrink: 0; font-size: 10px; }
  .log-icon { flex-shrink: 0; width: 16px; text-align: center; font-size: 10px; }
  .log-text { word-break: break-word; }

  .log.t-text .log-text { color: #c9d1d9; }
  .log.t-tool .log-text { color: #d2a8ff; }
  .log.t-patch .log-text { color: #3fb950; }
  .log.t-reasoning .log-text { color: #8b949e; font-style: italic; }
  .log.t-step-start .log-text { color: #58a6ff; }
  .log.t-step-finish .log-text { color: #484f58; }

  .card-footer {
    padding: 8px 14px; border-top: 1px solid #21262d;
    display: flex; gap: 6px; align-items: center;
  }
  .card-footer .tag {
    font-size: 10px; padding: 2px 8px; border-radius: 10px;
    background: #1c2129; color: #8b949e; border: 1px solid #30363d;
  }

  /* --- Detail panel (overlay) --- */
  .detail-overlay {
    display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.6);
    z-index: 100; justify-content: center; align-items: center;
  }
  .detail-overlay.open { display: flex; }
  .detail-panel {
    background: #0d1117; border: 1px solid #30363d; border-radius: 12px;
    width: 720px; max-height: 80vh; display: flex; flex-direction: column;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
  }
  .detail-header {
    padding: 16px 20px; border-bottom: 1px solid #21262d;
    display: flex; align-items: center; justify-content: space-between;
  }
  .detail-header h2 { font-size: 14px; font-weight: 600; }
  .detail-close {
    background: none; border: none; color: #8b949e; font-size: 18px;
    cursor: pointer; padding: 4px 8px;
  }
  .detail-close:hover { color: #e6edf3; }
  .detail-logs {
    flex: 1; overflow-y: auto; padding: 12px 0; font-family: 'SF Mono', monospace;
  }
  .detail-logs .log { padding: 3px 20px; font-size: 12px; }

  .empty { padding: 40px; text-align: center; color: #484f58; font-size: 13px; }
</style>
</head>
<body>

<div class="header">
  <h1>aproc</h1>
  <span class="dot"></span>
  <span class="status" id="status">connecting...</span>
</div>

<div class="board" id="board">
  <div class="empty">Loading sessions...</div>
</div>

<div class="detail-overlay" id="detail-overlay">
  <div class="detail-panel">
    <div class="detail-header">
      <h2 id="detail-title">Session</h2>
      <button class="detail-close" id="detail-close">&times;</button>
    </div>
    <div class="detail-logs" id="detail-logs"></div>
  </div>
</div>

<script>
const ICONS = {
  'text':'💬','tool':'🔧','patch':'📝','reasoning':'🧠',
  'step-start':'▶','step-finish':'■','compaction':'📦'
};

// Track recent activity per session (for live detection)
const sessionLastSeen = {};
let ws;

// --- Render board ---
async function loadBoard() {
  const res = await fetch('/api/sessions');
  const sessions = await res.json();
  const board = document.getElementById('board');
  if (!sessions.length) { board.innerHTML = '<div class="empty">No sessions</div>'; return; }

  board.innerHTML = sessions.map(s => {
    const isLive = sessionLastSeen[s.id] && (Date.now() - sessionLastSeen[s.id] < 30000);
    const status = isLive ? 'running' : (s.last_activity ? 'idle' : 'no activity');
    return `
      <div class="card ${isLive ? 'live' : ''}" data-id="${s.id}">
        <div class="card-header">
          <div class="card-indicator"></div>
          <div class="card-info">
            <div class="card-title">${esc(s.title || s.slug || s.id.slice(0,12))}</div>
            <div class="card-meta">${s.part_count} parts · ${timeAgo(s.last_activity)}</div>
            <div class="card-status">${status}</div>
          </div>
        </div>
        <div class="card-logs" id="logs-${s.id}"></div>
        <div class="card-footer">
          <span class="tag">${esc(s.directory ? s.directory.split('/').pop() : '?')}</span>
          <span class="tag">${esc(s.slug || '')}</span>
        </div>
      </div>`;
  }).join('');

  // Load recent logs for each card
  for (const s of sessions.slice(0, 12)) {
    loadCardLogs(s.id);
  }

  // Click to open detail
  board.querySelectorAll('.card').forEach(card => {
    card.addEventListener('click', () => openDetail(card.dataset.id,
      card.querySelector('.card-title').textContent));
  });
}

async function loadCardLogs(sessionId) {
  const el = document.getElementById('logs-' + sessionId);
  if (!el) return;
  const res = await fetch(`/api/sessions/${sessionId}/parts?limit=15`);
  const parts = await res.json();
  el.innerHTML = parts.map(p => logHTML(p)).join('');
  el.scrollTop = el.scrollHeight;
}

function logHTML(p) {
  let text = '';
  switch(p.type) {
    case 'text': text = (p.text||'').slice(0,120); break;
    case 'tool': text = `${p.tool_name||'?'} ${p.state?'('+p.state+')':''} ${(p.tool_description||'').slice(0,60)}`; break;
    case 'patch': text = p.path||''; break;
    case 'reasoning': text = (p.text||'').slice(0,100); break;
    case 'step-start': text = 'step started'; break;
    case 'step-finish': text = 'step finished'; break;
    default: text = p.type;
  }
  return `<div class="log t-${p.type}">
    <span class="log-time">${p.time_str}</span>
    <span class="log-icon">${ICONS[p.type]||'❓'}</span>
    <span class="log-text">${esc(text)}</span>
  </div>`;
}

// --- Detail panel ---
async function openDetail(id, title) {
  document.getElementById('detail-title').textContent = title;
  const logs = document.getElementById('detail-logs');
  logs.innerHTML = '<div class="empty">Loading...</div>';
  document.getElementById('detail-overlay').classList.add('open');

  const res = await fetch(`/api/sessions/${id}/parts?limit=500`);
  const parts = await res.json();
  logs.innerHTML = parts.map(p => logHTML(p)).join('');
  logs.scrollTop = logs.scrollHeight;

  // Live updates into detail
  window._detailSession = id;
}

document.getElementById('detail-close').addEventListener('click', () => {
  document.getElementById('detail-overlay').classList.remove('open');
  window._detailSession = null;
});
document.getElementById('detail-overlay').addEventListener('click', (e) => {
  if (e.target === e.currentTarget) {
    e.currentTarget.classList.remove('open');
    window._detailSession = null;
  }
});

// --- WebSocket ---
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(`${proto}//${location.host}/ws`);

  ws.onopen = () => {
    document.getElementById('status').textContent = 'live';
    document.querySelector('.dot').style.background = '#3fb950';
  };

  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    if (msg.type !== 'part') return;
    const p = msg.data;

    // Track live sessions
    sessionLastSeen[p.session_id] = Date.now();

    // Update card live status
    const card = document.querySelector(`.card[data-id="${p.session_id}"]`);
    if (card && !card.classList.contains('live')) {
      card.classList.add('live');
      card.querySelector('.card-status').textContent = 'running';
    }

    // Append to card mini-log
    const cardLogs = document.getElementById('logs-' + p.session_id);
    if (cardLogs) {
      cardLogs.insertAdjacentHTML('beforeend', logHTML(p));
      // Keep max 20 entries in card
      while (cardLogs.children.length > 20) cardLogs.firstChild.remove();
      cardLogs.scrollTop = cardLogs.scrollHeight;
    }

    // Append to detail panel if open
    if (window._detailSession === p.session_id) {
      const dl = document.getElementById('detail-logs');
      dl.insertAdjacentHTML('beforeend', logHTML(p));
      if (dl.scrollHeight - dl.scrollTop - dl.clientHeight < 100) {
        dl.scrollTop = dl.scrollHeight;
      }
    }
  };

  ws.onclose = () => {
    document.getElementById('status').textContent = 'reconnecting...';
    document.querySelector('.dot').style.background = '#f85149';
    setTimeout(connectWS, 2000);
  };
}

// --- Check live status decay ---
setInterval(() => {
  document.querySelectorAll('.card.live').forEach(card => {
    const id = card.dataset.id;
    if (!sessionLastSeen[id] || Date.now() - sessionLastSeen[id] > 30000) {
      card.classList.remove('live');
      card.querySelector('.card-status').textContent = 'idle';
    }
  });
}, 5000);

// --- Utils ---
function esc(s) { const d = document.createElement('div'); d.textContent = s||''; return d.innerHTML; }
function timeAgo(ts) {
  if (!ts) return 'no activity';
  const diff = Date.now() - ts;
  if (diff < 60000) return 'just now';
  if (diff < 3600000) return Math.floor(diff/60000) + 'm ago';
  if (diff < 86400000) return Math.floor(diff/3600000) + 'h ago';
  return Math.floor(diff/86400000) + 'd ago';
}

// --- Init ---
loadBoard();
connectWS();
setInterval(loadBoard, 30000);
</script>
</body>
</html>
"""
