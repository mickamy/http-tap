const events = [];
let selectedIdx = -1;
let filterText = '';
let autoScroll = true;
let viewMode = 'events';
let statsSortKey = 'total';
let statsSortAsc = false;
let selectedStatsEndpoint = null;
let paused = false;

const tbody = document.getElementById('tbody');
const tableWrap = document.getElementById('table-wrap');
const statsWrap = document.getElementById('stats-wrap');
const statsTbody = document.getElementById('stats-tbody');
const statsEl = document.getElementById('stats');
const statusEl = document.getElementById('status');
const filterEl = document.getElementById('filter');
const detailEl = document.getElementById('detail');
const statsDetailEl = document.getElementById('stats-detail');
const replayOutput = document.getElementById('replay-output');

filterEl.addEventListener('input', () => {
  filterText = filterEl.value;
  render();
});

tableWrap.addEventListener('scroll', () => {
  const el = tableWrap;
  autoScroll = el.scrollTop + el.clientHeight >= el.scrollHeight - 20;
});

document.querySelectorAll('#stats-wrap th.sortable').forEach(th => {
  th.addEventListener('click', () => {
    const key = th.dataset.sort;
    if (statsSortKey === key) {
      statsSortAsc = !statsSortAsc;
    } else {
      statsSortKey = key;
      statsSortAsc = false;
    }
    document.querySelectorAll('#stats-wrap th.sortable').forEach(h => h.classList.remove('active'));
    th.classList.add('active');
    renderStats();
  });
});

function switchView(mode) {
  viewMode = mode;
  document.getElementById('tab-events').classList.toggle('active', mode === 'events');
  document.getElementById('tab-stats').classList.toggle('active', mode === 'stats');
  tableWrap.style.display = mode === 'events' ? '' : 'none';
  statsWrap.style.display = mode === 'stats' ? '' : 'none';
  if (mode === 'events') {
    detailEl.className = selectedIdx >= 0 ? 'open' : '';
    statsDetailEl.className = '';
  } else {
    detailEl.className = '';
    statsDetailEl.className = selectedStatsEndpoint ? 'open' : '';
  }
  render();
}

let renderPending = false;
function render() {
  if (renderPending) return;
  renderPending = true;
  requestAnimationFrame(() => {
    renderPending = false;
    if (viewMode === 'events') {
      renderTable();
    } else {
      renderStats();
    }
  });
}

// Filter parsing
const RE_DURATION = /^d([><])(\d+(?:\.\d+)?)(us|\u00b5s|ms|s|m)$/;
const RE_STATUS = /^s([><]?=?)(\d+)$/;

function parseFilterTokens(input) {
  if (!input.trim()) return [];
  return input.trim().split(/\s+/).map(tok => {
    const dm = RE_DURATION.exec(tok);
    if (dm) {
      const op = dm[1];
      const val = parseFloat(dm[2]);
      const unit = dm[3];
      let ms;
      switch (unit) {
        case 'us': case '\u00b5s': ms = val / 1000; break;
        case 'ms': ms = val; break;
        case 's': ms = val * 1000; break;
        case 'm': ms = val * 60000; break;
        default: ms = val;
      }
      return {kind: 'duration', op, ms};
    }
    const sm = RE_STATUS.exec(tok);
    if (sm) {
      return {kind: 'status', op: sm[1] || '=', code: parseInt(sm[2], 10)};
    }
    if (tok.toLowerCase() === 'error') return {kind: 'error'};
    return {kind: 'text', text: tok.toLowerCase()};
  });
}

function matchesFilter(ev, cond) {
  switch (cond.kind) {
    case 'duration':
      return cond.op === '>' ? ev.duration_ms > cond.ms : ev.duration_ms < cond.ms;
    case 'status':
      switch (cond.op) {
        case '>=': return ev.status >= cond.code;
        case '<=': return ev.status <= cond.code;
        case '>': return ev.status > cond.code;
        case '<': return ev.status < cond.code;
        default: return ev.status === cond.code;
      }
    case 'error':
      return ev.status >= 400;
    case 'text':
      return (ev.method || '').toLowerCase().includes(cond.text) ||
             (ev.path || '').toLowerCase().includes(cond.text) ||
             (ev.error && ev.error.toLowerCase().includes(cond.text));
  }
  return false;
}

function getFiltered() {
  const conds = parseFilterTokens(filterText);
  if (conds.length === 0) return events.map((ev, i) => ({ev, idx: i}));
  return events.reduce((acc, ev, i) => {
    if (conds.every(c => matchesFilter(ev, c))) acc.push({ev, idx: i});
    return acc;
  }, []);
}

function fmtDur(ms) {
  if (ms < 1) return (ms * 1000).toFixed(0) + '\u00b5s';
  if (ms < 1000) return ms.toFixed(1) + 'ms';
  return (ms / 1000).toFixed(2) + 's';
}

function fmtTime(iso) {
  const d = new Date(iso);
  return d.toLocaleTimeString('en-GB', {hour12: false}) + '.' + String(d.getMilliseconds()).padStart(3, '0');
}

function escapeHTML(s) {
  const el = document.createElement('span');
  el.textContent = s;
  return el.innerHTML;
}

function statusClass(status) {
  if (status >= 500) return 'status-5xx';
  if (status >= 400) return 'status-4xx';
  if (status >= 300) return 'status-3xx';
  if (status >= 200) return 'status-2xx';
  return '';
}

function methodClass(method) {
  return 'method-' + (method || '').toLowerCase();
}

function renderTable() {
  const filtered = getFiltered();
  const hasFilter = filterText.trim().length > 0;
  const pauseLabel = paused ? ' (paused)' : '';
  const eventCount = hasFilter
    ? filtered.length + '/' + events.length
    : String(events.length);
  statsEl.textContent = `${eventCount} requests${pauseLabel}`;

  const fragment = document.createDocumentFragment();
  for (const {ev, idx} of filtered) {
    const tr = document.createElement('tr');
    tr.className = 'row' +
      (idx === selectedIdx ? ' selected' : '') +
      (ev.status >= 400 ? ' has-error' : '');
    tr.dataset.idx = idx;
    tr.onclick = () => selectRow(idx);
    const sc = statusClass(ev.status);
    const mc = methodClass(ev.method);
    tr.innerHTML =
      `<td class="col-time">${escapeHTML(fmtTime(ev.start_time))}</td>` +
      `<td class="col-method-verb"><span class="${mc}">${escapeHTML(ev.method)}</span></td>` +
      `<td class="col-path" title="${escapeHTML(ev.path)}">${escapeHTML(ev.path)}</td>` +
      `<td class="col-status"><span class="${sc}">${ev.status}</span></td>` +
      `<td class="col-dur">${escapeHTML(fmtDur(ev.duration_ms))}</td>`;
    fragment.appendChild(tr);
  }
  tbody.replaceChildren(fragment);

  if (autoScroll && selectedIdx < 0) {
    tableWrap.scrollTop = tableWrap.scrollHeight;
  }
}

// --- Stats view ---

function endpointKey(ev) {
  // Strip query string for grouping.
  const path = (ev.path || '').split('?')[0];
  return ev.method + ' ' + path;
}

function buildStats() {
  const groups = new Map();
  const textConds = parseFilterTokens(filterText).filter(c => c.kind === 'text');
  for (const ev of events) {
    const key = endpointKey(ev);
    if (textConds.length > 0 && !textConds.every(c => key.toLowerCase().includes(c.text))) continue;
    let group = groups.get(key);
    if (!group) {
      group = {endpoint: key, durations: [], errors: 0};
      groups.set(key, group);
    }
    group.durations.push(ev.duration_ms);
    if (ev.status >= 400) group.errors++;
  }
  const rows = [];
  for (const g of groups.values()) {
    const durs = g.durations.sort((a, b) => a - b);
    const count = durs.length;
    const total = durs.reduce((s, d) => s + d, 0);
    const avg = total / count;
    rows.push({endpoint: g.endpoint, count, errors: g.errors, avg, total});
  }
  return rows;
}

function sortStats(rows) {
  const dir = statsSortAsc ? 1 : -1;
  rows.sort((a, b) => {
    let va, vb;
    if (statsSortKey === 'errors') {
      va = a.count > 0 ? a.errors / a.count : 0;
      vb = b.count > 0 ? b.errors / b.count : 0;
    } else {
      va = a[statsSortKey];
      vb = b[statsSortKey];
    }
    if (va < vb) return -1 * dir;
    if (va > vb) return 1 * dir;
    return 0;
  });
}

function renderStats() {
  const rows = buildStats();
  sortStats(rows);
  statsEl.textContent = `${rows.length} endpoints`;

  const fragment = document.createDocumentFragment();
  for (const r of rows) {
    const tr = document.createElement('tr');
    tr.className = 'row' + (selectedStatsEndpoint === r.endpoint ? ' selected' : '');
    tr.onclick = () => selectStatsRow(r);
    const errStr = r.errors > 0
      ? `<span class="status-err">${r.errors}(${(r.errors / r.count * 100).toFixed(0)}%)</span>`
      : '0';
    tr.innerHTML =
      `<td class="stats-col-count">${r.count}</td>` +
      `<td class="stats-col-errors">${errStr}</td>` +
      `<td class="stats-col-dur">${fmtDur(r.avg)}</td>` +
      `<td class="stats-col-dur">${fmtDur(r.total)}</td>` +
      `<td class="stats-col-endpoint" title="${escapeHTML(r.endpoint)}">${escapeHTML(r.endpoint)}</td>`;
    fragment.appendChild(tr);
  }
  statsTbody.replaceChildren(fragment);
}

function selectStatsRow(r) {
  if (selectedStatsEndpoint === r.endpoint) {
    selectedStatsEndpoint = null;
    statsDetailEl.className = '';
    renderStats();
    return;
  }
  selectedStatsEndpoint = r.endpoint;

  const errStr = r.errors > 0
    ? `${r.errors} (${(r.errors / r.count * 100).toFixed(0)}%)`
    : '0';
  document.getElementById('sd-metrics').innerHTML =
    `<span class="detail-label">Count:</span><span class="detail-value">${r.count}</span>` +
    `<span class="detail-label" style="margin-left:12px">Errors:</span><span class="detail-value">${errStr}</span>` +
    `<span class="detail-label" style="margin-left:12px">Avg:</span><span class="detail-value">${fmtDur(r.avg)}</span>` +
    `<span class="detail-label" style="margin-left:12px">Total:</span><span class="detail-value">${fmtDur(r.total)}</span>`;
  document.getElementById('sd-method').textContent = r.endpoint;
  statsDetailEl.className = 'open';
  renderStats();
}

function copyStatsMethod() {
  if (!selectedStatsEndpoint) return;
  copyToClipboard(selectedStatsEndpoint);
}

function selectRow(idx) {
  if (selectedIdx === idx) {
    selectedIdx = -1;
    detailEl.className = '';
    renderTable();
    return;
  }
  selectedIdx = idx;
  const ev = events[idx];
  document.getElementById('d-method').textContent = ev.method;
  document.getElementById('d-path').textContent = ev.path;
  document.getElementById('d-time').textContent = fmtTime(ev.start_time);
  document.getElementById('d-dur').textContent = fmtDur(ev.duration_ms);

  const statusDisplay = document.getElementById('d-status');
  statusDisplay.textContent = ev.status;
  statusDisplay.className = 'detail-value ' + statusClass(ev.status);

  const errRow = document.getElementById('d-err-row');
  if (ev.error) {
    document.getElementById('d-err').textContent = ev.error;
    errRow.style.display = '';
  } else {
    errRow.style.display = 'none';
  }

  const reqHeaders = ev.request_headers || {};
  const resHeaders = ev.response_headers || {};
  document.getElementById('d-req-headers').textContent = formatHeaders(reqHeaders);
  document.getElementById('d-res-headers').textContent = formatHeaders(resHeaders);
  document.getElementById('d-req-headers-section').style.display = Object.keys(reqHeaders).length > 0 ? '' : 'none';
  document.getElementById('d-res-headers-section').style.display = Object.keys(resHeaders).length > 0 ? '' : 'none';

  const reqBody = ev.request_body || '';
  const resBody = ev.response_body || '';
  document.getElementById('d-req-body').textContent = reqBody ? decodeBody(reqBody) : '';
  document.getElementById('d-res-body').textContent = resBody ? decodeBody(resBody) : '';
  document.getElementById('d-req-body-section').style.display = reqBody ? '' : 'none';
  document.getElementById('d-res-body-section').style.display = resBody ? '' : 'none';

  document.querySelectorAll('.detail-pre').forEach(el => el.classList.add('collapsed'));
  document.querySelectorAll('.section-chevron').forEach(el => el.textContent = '\u25b8');

  replayOutput.className = '';
  detailEl.className = 'open';
  renderTable();
}

function formatHeaders(headers) {
  const keys = Object.keys(headers).sort();
  return keys.map(k => k + ': ' + headers[k]).join('\n');
}

function toggleSection(name) {
  const el = document.getElementById('d-' + name);
  const chevron = document.getElementById('chevron-' + name);
  if (el.classList.contains('collapsed')) {
    el.classList.remove('collapsed');
    chevron.textContent = '\u25be';
  } else {
    el.classList.add('collapsed');
    chevron.textContent = '\u25b8';
  }
}

// --- Body decoding ---

function decodeBody(b64) {
  try {
    const binary = atob(b64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    const text = new TextDecoder('utf-8', {fatal: true}).decode(bytes);
    try {
      const obj = JSON.parse(text);
      return JSON.stringify(obj, null, 2);
    } catch (_) {}
    return text;
  } catch (_) {
    try {
      const binary = atob(b64);
      return hexDump(binary);
    } catch (_) {
      return b64;
    }
  }
}

function hexDump(str) {
  const lines = [];
  for (let i = 0; i < str.length; i += 16) {
    const hex = [];
    let ascii = '';
    for (let j = 0; j < 16; j++) {
      if (i + j < str.length) {
        const c = str.charCodeAt(i + j);
        hex.push(c.toString(16).padStart(2, '0'));
        ascii += (c >= 0x20 && c < 0x7f) ? str[i + j] : '.';
      } else {
        hex.push('  ');
      }
    }
    const addr = i.toString(16).padStart(8, '0');
    lines.push(addr + '  ' + hex.slice(0, 8).join(' ') + '  ' + hex.slice(8).join(' ') + '  |' + ascii + '|');
  }
  return lines.join('\n');
}

// --- Copy ---

function copyBody(which) {
  if (selectedIdx < 0) return;
  const ev = events[selectedIdx];
  const b64 = which === 'request' ? ev.request_body : ev.response_body;
  if (!b64) return;
  copyToClipboard(decodeBody(b64));
}

function copyToClipboard(text) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => showToast('Copied!')).catch(() => fallbackCopy(text));
  } else {
    fallbackCopy(text);
  }
}

function fallbackCopy(text) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.select();
  document.execCommand('copy');
  document.body.removeChild(ta);
  showToast('Copied!');
}

// --- Replay ---

async function replayRequest() {
  if (selectedIdx < 0) return;
  const ev = events[selectedIdx];
  const pre = document.getElementById('replay-pre');
  pre.textContent = 'Replaying...';
  pre.className = '';
  replayOutput.className = 'open';

  try {
    const resp = await fetch('/api/replay', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        method: ev.method,
        path: ev.path,
        headers: ev.request_headers || {},
        body: ev.request_body || '',
      }),
    });
    const data = await resp.json();
    if (data.error) {
      pre.textContent = data.error;
      pre.className = 'replay-error';
    } else if (data.event) {
      const e = data.event;
      let output = `Status: ${e.status}\nDuration: ${fmtDur(e.duration_ms)}`;
      if (e.error) output += `\nError: ${e.error}`;
      if (e.response_body) {
        output += '\n\nResponse Body:\n' + decodeBody(e.response_body);
      }
      pre.textContent = output;
      pre.className = '';
    }
  } catch (e) {
    pre.textContent = 'Request failed: ' + e.message;
    pre.className = 'replay-error';
  }
}

// --- Toast ---

function showToast(msg) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.classList.add('show');
  setTimeout(() => t.classList.remove('show'), 2000);
}

// --- Controls ---

function togglePause() {
  paused = !paused;
  const btn = document.getElementById('btn-pause');
  btn.textContent = paused ? 'Resume' : 'Pause';
  btn.classList.toggle('active', paused);
  render();
}

function clearEvents() {
  events.length = 0;
  selectedIdx = -1;
  selectedStatsEndpoint = null;
  detailEl.className = '';
  statsDetailEl.className = '';
  render();
}

// --- Export ---

function exportData(format) {
  const data = buildExportData();
  const content = format === 'json' ? renderExportJSON(data) : renderExportMarkdown(data);
  const ext = format === 'json' ? 'json' : 'md';
  const now = new Date();
  const ts = now.getFullYear().toString() +
    String(now.getMonth() + 1).padStart(2, '0') +
    String(now.getDate()).padStart(2, '0') + '-' +
    String(now.getHours()).padStart(2, '0') +
    String(now.getMinutes()).padStart(2, '0') +
    String(now.getSeconds()).padStart(2, '0');
  downloadBlob(content, `http-tap-${ts}.${ext}`);
}

function buildExportData() {
  const filtered = getFiltered();
  const exported = filtered.map(f => f.ev);

  let periodStart = '';
  let periodEnd = '';
  if (exported.length > 0) {
    periodStart = fmtTimeHMS(exported[0].start_time);
    periodEnd = fmtTimeHMS(exported[exported.length - 1].start_time);
  }

  const calls = exported.map(ev => ({
    time: fmtTime(ev.start_time),
    method: ev.method,
    path: ev.path,
    duration_ms: ev.duration_ms,
    status: ev.status,
    error: ev.error || '',
  }));

  return {
    captured: events.length,
    exported: exported.length,
    filter: filterText,
    period: {start: periodStart, end: periodEnd},
    calls,
    analytics: buildExportAnalytics(exported),
  };
}

function fmtTimeHMS(iso) {
  const d = new Date(iso);
  return String(d.getHours()).padStart(2, '0') + ':' +
    String(d.getMinutes()).padStart(2, '0') + ':' +
    String(d.getSeconds()).padStart(2, '0');
}

function buildExportAnalytics(exported) {
  const groups = new Map();
  const order = [];
  for (const ev of exported) {
    const key = endpointKey(ev);
    let g = groups.get(key);
    if (!g) {
      g = {durations: [], errors: 0};
      groups.set(key, g);
      order.push(key);
    }
    g.durations.push(ev.duration_ms);
    if (ev.status >= 400) g.errors++;
  }
  return order.map(endpoint => {
    const g = groups.get(endpoint);
    const durs = g.durations.slice().sort((a, b) => a - b);
    const count = durs.length;
    const total = durs.reduce((s, d) => s + d, 0);
    const avg = total / count;
    const p95 = durs[Math.floor((count - 1) * 0.95)];
    const mx = durs[count - 1];
    return {endpoint, count, errors: g.errors, total_ms: total, avg_ms: avg, p95_ms: p95, max_ms: mx};
  });
}

function fmtDurExport(ms) {
  if (ms < 1) return Math.round(ms * 1000) + '\u00b5s';
  if (ms < 1000) return ms.toFixed(1) + 'ms';
  return (ms / 1000).toFixed(2) + 's';
}

function renderExportJSON(data) {
  return JSON.stringify(data, null, '  ') + '\n';
}

function renderExportMarkdown(data) {
  let md = '# http-tap export\n\n';
  md += `- Captured: ${data.captured} requests\n`;
  let exportLine = `- Exported: ${data.exported} requests`;
  if (data.filter) {
    exportLine += ` (filter: ${escPipe(data.filter)})`;
  }
  md += exportLine + '\n';
  if (data.period.start) {
    md += `- Period: ${data.period.start} \u2014 ${data.period.end}\n`;
  }

  md += '\n## Requests\n\n';
  md += '| # | Time | Method | Path | Status | Duration | Error |\n';
  md += '|---|------|--------|------|--------|----------|-------|\n';
  data.calls.forEach((c, i) => {
    md += `| ${i + 1} | ${c.time} | ${c.method} | ${escPipe(c.path)} | ${c.status} | ${fmtDurExport(c.duration_ms)} | ${escPipe(c.error)} |\n`;
  });

  if (data.analytics.length > 0) {
    md += '\n## Analytics\n\n';
    md += '| Endpoint | Count | Errors | Avg | P95 | Max | Total |\n';
    md += '|----------|-------|--------|-----|-----|-----|-------|\n';
    for (const a of data.analytics) {
      const errStr = a.errors > 0 ? `${a.errors}(${(a.errors / a.count * 100).toFixed(0)}%)` : '0';
      md += `| ${escPipe(a.endpoint)} | ${a.count} | ${errStr} | ${fmtDurExport(a.avg_ms)} | ${fmtDurExport(a.p95_ms)} | ${fmtDurExport(a.max_ms)} | ${fmtDurExport(a.total_ms)} |\n`;
    }
  }

  return md;
}

function escPipe(s) {
  return (s || '').replace(/\|/g, '\\|');
}

function downloadBlob(content, filename) {
  const blob = new Blob([content], {type: 'text/plain'});
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

// --- SSE ---

function connectSSE() {
  const es = new EventSource('/api/events');
  es.onopen = () => {
    statusEl.textContent = 'connected';
    statusEl.className = 'status connected';
  };
  es.onmessage = (e) => {
    if (paused) return;
    const ev = JSON.parse(e.data);
    events.push(ev);
    render();
  };
  es.onerror = () => {
    statusEl.textContent = 'disconnected';
    statusEl.className = 'status disconnected';
    es.close();
    setTimeout(connectSSE, 2000);
  };
}

connectSSE();
