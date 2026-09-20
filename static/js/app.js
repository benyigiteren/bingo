// Bingo — Stark Monochrome Interaction Engine

document.addEventListener('DOMContentLoaded', () => {
  // 1. Toast Notification System
  window.showToast = function(message, type = 'success') {
    let container = document.getElementById('toast-container');
    if (!container) {
      container = document.createElement('div');
      container.id = 'toast-container';
      document.body.appendChild(container);
    }

    const toast = document.createElement('div');
    toast.className = `toast ${type === 'error' ? 'alert-danger' : ''}`;
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transition = 'opacity 0.2s ease';
      setTimeout(() => toast.remove(), 200);
    }, 2800);
  };

  // 2. Clipboard Copy Utility
  window.copyText = function(text, label = 'Copied to clipboard!') {
    if (!navigator.clipboard) {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      document.body.appendChild(textarea);
      textarea.select();
      try {
        document.execCommand('copy');
        showToast(label);
      } catch (err) {
        showToast('Copy failed', 'error');
      }
      document.body.removeChild(textarea);
      return;
    }

    navigator.clipboard.writeText(text)
      .then(() => showToast(label))
      .catch(() => showToast('Copy failed', 'error'));
  };

  // 3. Tab Navigation
  const tabLinks = document.querySelectorAll('.tab-link');
  const tabSections = document.querySelectorAll('.tab-section');

  window.activateTab = function(targetId) {
    if (!targetId) targetId = '#files';
    tabLinks.forEach(link => {
      if (link.getAttribute('href') === targetId) {
        link.classList.add('active');
      } else {
        link.classList.remove('active');
      }
    });

    tabSections.forEach(section => {
      if ('#' + section.id === targetId) {
        section.classList.add('active');
      } else {
        section.classList.remove('active');
      }
    });
  };

  tabLinks.forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const targetId = link.getAttribute('href');
      history.pushState(null, '', targetId);
      activateTab(targetId);
    });
  });

  if (window.location.hash) {
    activateTab(window.location.hash);
  }

  // 4. Instant Search & Category Filters
  const searchInput = document.getElementById('file-search-input');
  const categoryPills = document.querySelectorAll('.category-pill');
  let currentFilter = 'all';

  function applyFilters() {
    const query = searchInput ? searchInput.value.toLowerCase().trim() : '';
    const pasteRows = document.querySelectorAll('.paste-row');

    function matchesCategory(filename, category) {
      if (category === 'all') return true;
      const ext = filename.substring(filename.lastIndexOf('.')).toLowerCase();
      if (category === 'code') {
        return ['.go', '.py', '.js', '.ts', '.jsx', '.tsx', '.rs', '.c', '.cpp', '.h', '.java', '.html', '.css', '.sh', '.sql', '.yaml', '.yml', '.json'].includes(ext);
      }
      if (category === 'text') {
        return ['.md', '.txt', '.log', '.env', '.ini', '.conf', '.csv'].includes(ext);
      }
      if (category === 'images') {
        return ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.svg'].includes(ext);
      }
      return true;
    }

    pasteRows.forEach(row => {
      const filename = row.dataset.filename || '';
      const textMatch = !query || filename.toLowerCase().includes(query);
      const catMatch = matchesCategory(filename, currentFilter);
      row.style.display = (textMatch && catMatch) ? 'flex' : 'none';
    });
  }

  if (searchInput) {
    searchInput.addEventListener('input', applyFilters);
  }

  categoryPills.forEach(btn => {
    btn.addEventListener('click', () => {
      categoryPills.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentFilter = btn.dataset.filter || 'all';
      applyFilters();
    });
  });

  // 5. Drag & Drop File Upload
  const uploadZone = document.getElementById('upload-zone');
  const fileInput = document.getElementById('file-input');

  if (uploadZone && fileInput) {
    uploadZone.addEventListener('click', () => fileInput.click());

    fileInput.addEventListener('change', () => {
      if (fileInput.files.length > 0) {
        uploadFiles(Array.from(fileInput.files));
      }
    });

    ['dragenter', 'dragover'].forEach(eventName => {
      uploadZone.addEventListener(eventName, (e) => {
        e.preventDefault();
        uploadZone.classList.add('dragover');
      }, false);
    });

    ['dragleave', 'drop'].forEach(eventName => {
      uploadZone.addEventListener(eventName, (e) => {
        e.preventDefault();
        uploadZone.classList.remove('dragover');
      }, false);
    });

    uploadZone.addEventListener('drop', (e) => {
      const dt = e.dataTransfer;
      const files = dt.files;
      if (files.length > 0) {
        uploadFiles(Array.from(files));
      }
    });
  }

  function uploadFiles(files) {
    if (files.length === 0) return;

    showToast(`Uploading ${files.length} file(s)...`);
    let completed = 0;

    files.forEach(file => {
      const formData = new FormData();
      formData.append('file', file);

      const csrfInput = document.querySelector('input[name="csrf_token"]');
      if (csrfInput) {
        formData.append('csrf_token', csrfInput.value);
      }

      fetch('/dashboard/upload', {
        method: 'POST',
        body: formData
      })
      .then(res => res.json())
      .then(data => {
        completed++;
        if (data.success) {
          if (completed === files.length) {
            showToast('Upload complete!');
            setTimeout(() => window.location.reload(), 600);
          }
        } else {
          showToast(data.error || `${file.name} failed`, 'error');
        }
      })
      .catch(err => {
        showToast(`Error: ${file.name}`, 'error');
      });
    });
  }

  // 6. Global Clipboard Listener (Ctrl+V)
  document.addEventListener('paste', (e) => {
    const active = document.activeElement;
    if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA' || active.isContentEditable)) {
      return;
    }

    const items = (e.clipboardData || e.originalEvent.clipboardData).items;
    let foundImage = false;

    for (let i = 0; i < items.length; i++) {
      if (items[i].type.indexOf('image') !== -1) {
        const blob = items[i].getAsFile();
        if (blob) {
          foundImage = true;
          const ext = blob.type.split('/')[1] || 'png';
          const file = new File([blob], `clipboard_${Date.now()}.${ext}`, { type: blob.type });
          uploadFiles([file]);
          break;
        }
      }
    }

    if (!foundImage) {
      const text = e.clipboardData.getData('text');
      if (text && text.trim().length > 0) {
        activateTab('#editor');
        const editorTextarea = document.getElementById('editor_content');
        if (editorTextarea) {
          editorTextarea.value = text;
          editorTextarea.focus();
          showToast('Pasted into editor');
          updateMarkdownPreview(text);
        }
      }
    }
  });

  // 7. Split Editor Live Markdown Preview & Tab Key Support
  const editorContent = document.getElementById('editor_content');
  const editorPreview = document.getElementById('editor_preview');
  const editorLangSelect = document.getElementById('editor_language');
  const editorFilename = document.getElementById('editor_filename');

  function updateMarkdownPreview(text) {
    if (!editorPreview) return;
    if (window.renderSimpleMarkdown) {
      editorPreview.innerHTML = window.renderSimpleMarkdown(text);
    } else {
      editorPreview.textContent = text;
    }
  }

  if (editorContent) {
    editorContent.addEventListener('input', (e) => {
      updateMarkdownPreview(e.target.value);
    });

    editorContent.addEventListener('keydown', (e) => {
      if (e.key === 'Tab') {
        e.preventDefault();
        const start = editorContent.selectionStart;
        const end = editorContent.selectionEnd;
        editorContent.value = editorContent.value.substring(0, start) + '  ' + editorContent.value.substring(end);
        editorContent.selectionStart = editorContent.selectionEnd = start + 2;
      }
    });
  }

  if (editorLangSelect && editorFilename) {
    editorLangSelect.addEventListener('change', () => {
      const ext = editorLangSelect.value;
      if (ext) {
        let current = editorFilename.value.trim();
        const dotIdx = current.lastIndexOf('.');
        if (dotIdx !== -1) {
          current = current.substring(0, dotIdx);
        }
        if (!current) current = 'paste_' + Date.now();
        editorFilename.value = current + ext;
      }
    });
  }

  // 8. QR Code Modal (Monochrome Minimalist)
  window.openQRModal = function(url, filename) {
    let modal = document.getElementById('qr-modal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'qr-modal';
      modal.className = 'modal-backdrop';
      modal.innerHTML = `
        <div class="modal-window">
          <div class="modal-title-bar">
            <span>share via qr</span>
            <button onclick="closeQRModal()" style="background:none; border:none; color:var(--text-muted); cursor:pointer; font-family:var(--font-mono); font-size:14px;">[x]</button>
          </div>
          <div style="text-align: center;">
            <div style="font-family: var(--font-mono); font-size: 11.5px; color: var(--text-secondary); margin-bottom: 12px;" id="qr-modal-filename"></div>
            <div id="qr-code-target" style="display: inline-block; background: #fff; padding: 12px; border-radius: var(--radius);"></div>
            <div style="margin-top: 12px; font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); word-break: break-all;" id="qr-modal-url"></div>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; border-top: 1px solid var(--border); padding-top: 12px;">
            <button class="btn btn-sm btn-primary" onclick="copyText(window.currentQRUrl, 'Link copied!')">copy link</button>
            <button class="btn btn-sm" onclick="closeQRModal()">close</button>
          </div>
        </div>
      `;
      document.body.appendChild(modal);
    }

    window.currentQRUrl = url;
    document.getElementById('qr-modal-filename').textContent = filename;
    document.getElementById('qr-modal-url').textContent = url;

    const target = document.getElementById('qr-code-target');
    const qrImg = document.createElement('img');
    qrImg.src = `https://api.qrserver.com/v1/create-qr-code/?size=160x160&data=${encodeURIComponent(url)}`;
    qrImg.width = 160;
    qrImg.height = 160;
    qrImg.alt = 'QR Code';
    target.innerHTML = '';
    target.appendChild(qrImg);

    modal.classList.add('active');
  };

  window.closeQRModal = function() {
    const modal = document.getElementById('qr-modal');
    if (modal) modal.classList.remove('active');
  };

  // 9. Safe Markdown Parser
  window.renderSimpleMarkdown = function(md) {
    if (!md) return '';
    let html = md
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");

    html = html.replace(/```(?:[a-zA-Z0-9_\-\.\+]+)?\n([\s\S]*?)```/g, '<pre><code>$1</code></pre>');
    html = html.replace(/`([^`\n]+)`/g, '<code>$1</code>');
    html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
    html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
    html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
    html = html.replace(/^&gt;[ \t]?(.*$)/gim, '<blockquote>$1</blockquote>');
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    html = html.replace(/^[ \t]*[\-\*][ \t](.*$)/gim, '<li>$1</li>');

    return `<div class="prose">${html}</div>`;
  };
});
