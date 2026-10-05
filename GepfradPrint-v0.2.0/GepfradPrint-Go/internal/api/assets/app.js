let selected = null;
const $ = (x) => document.querySelector(x);

async function api(u, o) {
  const r = await fetch(u, o);
  if (!r.ok) throw new Error(await r.text());
  return r.status === 204 ? null : r.json();
}

function selectedPrinter(printers) {
  return printers.find((p) => p.id === selected) || null;
}

async function loadPrinters() {
  const ps = await api('/api/printers');
  const keep = selectedPrinter(ps);
  if (!keep && ps.length) selected = ps[0].id;
  $('#printers').innerHTML = '';
  ps.forEach((p) => {
    const b = document.createElement('button');
    b.className = 'printer ' + (selected === p.id ? 'selected' : '');
    const endpoint = `${p.protocol || 'unknown'}://${p.host}:${p.port}`;
    b.textContent = `${p.name || p.host}\n${endpoint}`;
    b.onclick = () => { selected = p.id; renderPrinter(ps); };
    $('#printers').appendChild(b);
  });
  renderPrinter(ps);
}

function renderPrinter(ps) {
  const p = selectedPrinter(ps);
  const detail = $('#printer-detail');
  const diag = $('#diagnose');
  $('#diagnostics').innerHTML = '';
  if (!p) {
    detail.textContent = 'Select a printer.';
    diag.disabled = true;
    return;
  }
  diag.disabled = false;
  detail.innerHTML = `<div class="printer-title">${escapeHtml(p.name || p.host)}</div>` +
    `<div class="mono">Protocol: ${escapeHtml(p.protocol || 'unknown')} · Port: ${p.port || '?'} · Host: ${escapeHtml(p.host || '?')}</div>` +
    (p.address ? `<div class="mono">Address: ${escapeHtml(p.address)}</div>` : '') +
    (p.uri ? `<div class="mono">IPP URI: ${escapeHtml(p.uri)}</div>` : '');
}

function escapeHtml(s) {
  return String(s).replace(/[&<>'"]/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
}

async function scan() {
  const b = $('#discover');
  b.disabled = true;
  b.textContent = 'Scanning…';
  try {
    const found = await api('/api/discover');
    await loadPrinters();
    msg(`Discovery complete. Found ${found.length} printer service${found.length === 1 ? '' : 's'}.`);
  } catch (e) {
    msg(e.message, true);
  } finally {
    b.disabled = false;
    b.textContent = 'Scan for printers';
  }
}

async function diagnose() {
  if (!selected) return msg('Select a printer first.', true);
  const b = $('#diagnose');
  b.disabled = true;
  b.textContent = 'Testing…';
  $('#diagnostics').innerHTML = '<div class="diag pending">Running printer diagnostics…</div>';
  try {
    const result = await api('/api/diagnostics?id=' + encodeURIComponent(selected));
    showDiagnostics(result);
    msg('Diagnostics complete.');
  } catch (e) {
    $('#diagnostics').innerHTML = `<div class="diag fail">${escapeHtml(e.message)}</div>`;
    msg(e.message, true);
  } finally {
    b.disabled = false;
    b.textContent = 'Test printer connection';
  }
}

function showDiagnostics(result) {
  const d = result.diagnostics || {};
  const rows = [
    ['DNS / address', d.dns],
    ['TCP connection', d.tcp],
    ['TLS handshake', d.tls],
    ['IPP Get-Printer-Attributes', d.ipp]
  ];
  const parts = rows.map(([label, step]) => {
    const state = step && step.ok ? 'pass' : 'fail';
    return `<div class="diag-row"><span>${escapeHtml(label)}</span><b class="${state}">${step && step.ok ? 'PASS' : 'FAIL'}</b><code>${escapeHtml(step?.detail || 'not reached')}</code></div>`;
  });
  if (d.printer_state) parts.push(`<div class="diag-note"><b>Printer state:</b> ${escapeHtml(d.printer_state)}</div>`);
  if (d.document_formats?.length) parts.push(`<div class="diag-note"><b>Formats:</b> ${d.document_formats.map(escapeHtml).join(', ')}</div>`);
  if (d.media?.length) parts.push(`<div class="diag-note"><b>Media:</b> ${d.media.map(escapeHtml).join(', ')}</div>`);
  $('#diagnostics').innerHTML = `<div class="diag">${parts.join('')}</div>`;
}

async function send() {
  if (!selected) return msg('Select a printer first.', true);
  const f = $('#file').files[0];
  if (!f) return msg('Choose a file first.', true);
  const fd = new FormData();
  fd.append('file', f);
  fd.append('printer_id', selected);
  fd.append('copies', $('#copies').value);
  fd.append('media', $('#media').value);
  fd.append('orientation', $('#orientation').value);
  fd.append('color', $('#color').value);
  fd.append('duplex', $('#duplex').value);
  try {
    await api('/api/print', { method: 'POST', body: fd });
    msg('Job queued.');
    refresh();
  } catch (e) {
    msg(e.message, true);
  }
}

function msg(t, err = false) {
  $('#message').textContent = t;
  $('#message').className = err ? 'error' : '';
}

async function refresh() {
  try {
    const qs = await api('/api/queue');
    $('#queue').innerHTML = '';
    if (!qs.length) {
      $('#queue').innerHTML = '<p class="muted">No print jobs.</p>';
      return;
    }
    qs.sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
    qs.forEach((j) => {
      const row = document.createElement('div');
      row.className = 'job';
      const main = document.createElement('div');
      main.className = 'job-main';
      const name = document.createElement('b');
      name.textContent = j.file_name;
      const status = document.createElement('span');
      status.textContent = `${j.state} ${j.progress}%`;
      main.append(name, status);
      row.appendChild(main);
      if (j.error) {
        const err = document.createElement('div');
        err.className = 'job-error';
        err.textContent = j.error;
        row.appendChild(err);
      }
      const cancel = document.createElement('button');
      cancel.textContent = 'Cancel';
      cancel.onclick = () => cancelJob(j.id);
      row.appendChild(cancel);
      $('#queue').appendChild(row);
    });
  } catch (_) {}
}

async function cancelJob(id) {
  await fetch('/api/queue/' + encodeURIComponent(id), { method: 'DELETE' });
  refresh();
}

$('#discover').onclick = scan;
$('#diagnose').onclick = diagnose;
$('#print').onclick = send;
loadPrinters().catch((e) => msg(e.message, true));
refresh();
setInterval(refresh, 1000);
