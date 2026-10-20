// State & Utilities
let isRunning = false;

// Toast Notification
function showToast(message) {
  const toast = document.getElementById('toast');
  toast.textContent = message;
  toast.classList.add('show');
  setTimeout(() => {
    toast.classList.remove('show');
  }, 3000);
}

// Log into Terminal
function appendTerminalLog(msg, type = 'info') {
  const terminal = document.getElementById('terminal-logs');
  const entry = document.createElement('div');
  entry.className = `log-entry ${type}`;
  const timestamp = new Date().toLocaleTimeString();
  entry.textContent = `[${timestamp}] ${msg}`;
  terminal.appendChild(entry);
  terminal.scrollTop = terminal.scrollHeight;
}

// Tab Switching
document.querySelectorAll('.tab-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));

    btn.classList.add('active');
    const target = btn.getAttribute('data-tab');
    document.getElementById(target).classList.add('active');

    if (target === 'tab-contacts') loadContacts();
    if (target === 'tab-dlq') loadDLQ();
    if (target === 'tab-template') loadTemplate();
  });
});

// Insert Tag Token
window.insertToken = function(inputId, token) {
  const input = document.getElementById(inputId);
  const start = input.selectionStart;
  const end = input.selectionEnd;
  const text = input.value;
  input.value = text.substring(0, start) + token + text.substring(end);
  input.selectionStart = input.selectionEnd = start + token.length;
  input.focus();
  updateLivePreview();
};

// Live Preview Update
function updateLivePreview() {
  const subjectVal = document.getElementById('tpl-subject').value;
  const bodyVal = document.getElementById('tpl-body').value;

  const sampleName = 'Jane Doe';
  const sampleEmail = 'jane.doe@example.com';

  const previewSubject = subjectVal
    .replaceAll('{{.Name}}', sampleName)
    .replaceAll('{{.Email}}', sampleEmail);

  const previewBody = bodyVal
    .replaceAll('{{.Name}}', sampleName)
    .replaceAll('{{.Email}}', sampleEmail);

  document.getElementById('preview-subject').textContent = previewSubject || '(No Subject)';
  document.getElementById('preview-body').textContent = previewBody || '(No Body Content)';
}

document.getElementById('tpl-subject').addEventListener('input', updateLivePreview);
document.getElementById('tpl-body').addEventListener('input', updateLivePreview);

// Load Stats
async function loadStats() {
  try {
    const res = await fetch('/api/stats');
    if (!res.ok) return;
    const data = await res.json();
    document.getElementById('stat-total').textContent = data.total;
    document.getElementById('stat-sent').textContent = data.sent;
    document.getElementById('stat-pending').textContent = data.pending;
    document.getElementById('stat-failed').textContent = data.failed;
    document.getElementById('stat-dlq').textContent = data.dlq_count;
  } catch (err) {
    console.error('Error fetching stats:', err);
  }
}

// Load Template
async function loadTemplate() {
  try {
    const res = await fetch('/api/template');
    if (!res.ok) return;
    const data = await res.json();
    document.getElementById('tpl-subject').value = data.subject || '';
    document.getElementById('tpl-body').value = data.body || '';
    updateLivePreview();
  } catch (err) {
    showToast('Failed to load email template');
  }
}

// Save Template
document.getElementById('btn-save-template').addEventListener('click', async () => {
  const subject = document.getElementById('tpl-subject').value.trim();
  const body = document.getElementById('tpl-body').value.trim();

  try {
    const res = await fetch('/api/template', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ subject, body })
    });
    if (res.ok) {
      showToast('Template successfully updated & reloaded in memory!');
      appendTerminalLog('Template updated and reloaded.', 'success');
    } else {
      showToast('Error saving template');
    }
  } catch (err) {
    showToast('Failed to connect to server');
  }
});

// Load Contacts
async function loadContacts() {
  const tbody = document.getElementById('contacts-table-body');
  try {
    const res = await fetch('/api/contacts');
    if (!res.ok) throw new Error('API error');
    const contacts = await res.json();

    if (!contacts || contacts.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; color: var(--text-muted);">No contacts found in PostgreSQL database.</td></tr>';
      return;
    }

    tbody.innerHTML = contacts.map(c => {
      let badgeClass = 'badge-pending';
      if (c.status === 'SENT') badgeClass = 'badge-sent';
      if (c.status === 'FAILED') badgeClass = 'badge-failed';

      return `
        <tr>
          <td>#${c.id}</td>
          <td style="font-weight: 500;">${escapeHtml(c.name)}</td>
          <td style="color: var(--text-muted); font-family: monospace;">${escapeHtml(c.email)}</td>
          <td><span class="badge ${badgeClass}">${c.status}</span></td>
          <td style="color: #ef4444; font-size: 0.8rem;">${c.error_message || '-'}</td>
        </tr>
      `;
    }).join('');
  } catch (err) {
    tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; color: red;">Failed to load contacts from PostgreSQL.</td></tr>';
  }
}

// Add Contact Form
document.getElementById('form-add-contact').addEventListener('submit', async (e) => {
  e.preventDefault();
  const name = document.getElementById('add-name').value.trim();
  const email = document.getElementById('add-email').value.trim();

  try {
    const res = await fetch('/api/contacts', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, email })
    });
    if (res.ok) {
      showToast(`Added ${name} to contacts!`);
      document.getElementById('add-name').value = '';
      document.getElementById('add-email').value = '';
      loadContacts();
      loadStats();
    } else {
      const err = await res.text();
      showToast(`Error: ${err}`);
    }
  } catch (err) {
    showToast('Server connection failed');
  }
});

// Reset Contacts to Pending
document.getElementById('btn-reset-pending').addEventListener('click', async () => {
  if (!confirm('Are you sure you want to reset all contacts status back to PENDING?')) return;
  try {
    const res = await fetch('/api/contacts/reset', { method: 'POST' });
    if (res.ok) {
      showToast('All contacts reset to PENDING');
      appendTerminalLog('All contacts reset to PENDING status.', 'info');
      loadStats();
      loadContacts();
    }
  } catch (err) {
    showToast('Reset failed');
  }
});

// Load DLQ
async function loadDLQ() {
  const tbody = document.getElementById('dlq-table-body');
  try {
    const res = await fetch('/api/dlq');
    if (!res.ok) throw new Error('API error');
    const dlq = await res.json();

    if (!dlq || dlq.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; color: var(--text-muted);">Dead Letter Queue is empty! No failed messages.</td></tr>';
      return;
    }

    tbody.innerHTML = dlq.map(item => `
      <tr>
        <td style="font-weight: 500;">${escapeHtml(item.recipient.name)}</td>
        <td style="font-family: monospace;">${escapeHtml(item.recipient.email)}</td>
        <td>Worker #${item.worker_id}</td>
        <td style="color: #ef4444; font-size: 0.8rem;">${escapeHtml(item.error)}</td>
        <td style="color: var(--text-muted); font-size: 0.75rem;">${new Date(item.failed_at).toLocaleTimeString()}</td>
      </tr>
    `).join('');
  } catch (err) {
    tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; color: red;">Failed to load Redis DLQ messages.</td></tr>';
  }
}

// Clear DLQ
document.getElementById('btn-clear-dlq').addEventListener('click', async () => {
  try {
    const res = await fetch('/api/dlq/clear', { method: 'POST' });
    if (res.ok) {
      showToast('Redis DLQ cleared');
      loadDLQ();
      loadStats();
    }
  } catch (err) {
    showToast('Failed to clear DLQ');
  }
});

// Retry DLQ
document.getElementById('btn-retry-dlq').addEventListener('click', async () => {
  try {
    const res = await fetch('/api/dlq/retry', { method: 'POST' });
    if (res.ok) {
      const data = await res.json();
      showToast(`Re-queued ${data.count} items from DLQ back to PENDING!`);
      loadDLQ();
      loadStats();
      loadContacts();
    }
  } catch (err) {
    showToast('Failed to retry DLQ');
  }
});

// Start Campaign Trigger
async function triggerCampaign() {
  if (isRunning) {
    showToast('Campaign is already running!');
    return;
  }

  const startBtn = document.getElementById('btn-start-campaign');
  const quickBtn = document.getElementById('btn-quick-dispatch');
  startBtn.disabled = true;
  quickBtn.disabled = true;
  isRunning = true;

  appendTerminalLog('🚀 Dispatching campaign with 5 concurrent workers...', 'info');

  try {
    const res = await fetch('/api/campaign/start', { method: 'POST' });
    if (!res.ok) {
      const err = await res.text();
      appendTerminalLog(`❌ Failed to start campaign: ${err}`, 'error');
    } else {
      appendTerminalLog('Worker pool dispatched! Check Mailpit (localhost:8025) for live emails.', 'success');
      showToast('Campaign successfully started!');
    }
  } catch (err) {
    appendTerminalLog(`❌ Network error triggering campaign: ${err}`, 'error');
  } finally {
    setTimeout(() => {
      startBtn.disabled = false;
      quickBtn.disabled = false;
      isRunning = false;
      loadStats();
      loadContacts();
      loadDLQ();
    }, 2000);
  }
}

document.getElementById('btn-start-campaign').addEventListener('click', triggerCampaign);
document.getElementById('btn-quick-dispatch').addEventListener('click', triggerCampaign);

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/[&<>"']/g, m => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  })[m]);
}

// Initial Boot
loadStats();
loadTemplate();
loadContacts();
setInterval(loadStats, 3000);
