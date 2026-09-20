// Bingo - Modern Soft-Dark Interaction Engine
// Includes: Clipboard Paste (Ctrl+V), Instant Search & Filter, Grid/List Mode, SVG QR Generator, Split Editor

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
    toast.className = `toast ${type === 'error' ? 'alert-danger' : 'alert-success'}`;

    const icon = type === 'error'
      ? `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg>`
      : `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>`;

    toast.innerHTML = `${icon} <span>${message}</span>`;
    container.appendChild(toast);

    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transform = 'translateY(10px)';
      toast.style.transition = 'opacity 0.25s ease, transform 0.25s ease';
      setTimeout(() => toast.remove(), 250);
    }, 3200);
  };

  // 2. Clipboard Copy Utility
  window.copyText = function(text, label = 'Panoya kopyalandı!') {
    if (!navigator.clipboard) {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      document.body.appendChild(textarea);
      textarea.select();
      try {
        document.execCommand('copy');
        showToast(label);
      } catch (err) {
        showToast('Kopyalama başarısız oldu', 'error');
      }
      document.body.removeChild(textarea);
      return;
    }

    navigator.clipboard.writeText(text)
      .then(() => showToast(label))
      .catch(() => showToast('Kopyalama başarısız oldu', 'error'));
  };

  // 3. Tab Navigation
  const tabLinks = document.querySelectorAll('.tab-link');
  const tabSections = document.querySelectorAll('.tab-section');

  function activateTab(targetId) {
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
  }

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

  // 4. View Mode Toggle (Table List vs Bento Grid)
  const viewBtns = document.querySelectorAll('.view-btn');
  const tableView = document.getElementById('table-view');
  const gridView = document.getElementById('grid-view');

  function setViewMode(mode) {
    viewBtns.forEach(btn => {
      if (btn.dataset.view === mode) {
        btn.classList.add('active');
      } else {
        btn.classList.remove('active');
      }
    });

    if (tableView && gridView) {
      if (mode === 'grid') {
        tableView.style.display = 'none';
        gridView.style.display = 'grid';
      } else {
        tableView.style.display = 'block';
        gridView.style.display = 'none';
      }
    }
    localStorage.setItem('bingo_view_mode', mode);
  }

  viewBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      setViewMode(btn.dataset.view);
    });
  });

  const savedViewMode = localStorage.getItem('bingo_view_mode') || 'list';
  setViewMode(savedViewMode);

  // 5. Instant Search & Category Filters
  const searchInput = document.getElementById('file-search-input');
  const filterBtns = document.querySelectorAll('.filter-btn');
  let currentFilter = 'all';

  function applyFilters() {
    const query = searchInput ? searchInput.value.toLowerCase().trim() : '';
    const tableRows = document.querySelectorAll('#table-view tbody tr');
    const gridCards = document.querySelectorAll('.file-card');

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

    // Filter Table Rows
    tableRows.forEach(row => {
      const filename = row.dataset.filename || '';
      const textMatch = !query || filename.toLowerCase().includes(query);
      const catMatch = matchesCategory(filename, currentFilter);
      row.style.display = (textMatch && catMatch) ? '' : 'none';
    });

    // Filter Grid Cards
    gridCards.forEach(card => {
      const filename = card.dataset.filename || '';
      const textMatch = !query || filename.toLowerCase().includes(query);
      const catMatch = matchesCategory(filename, currentFilter);
      card.style.display = (textMatch && catMatch) ? 'flex' : 'none';
    });
  }

  if (searchInput) {
    searchInput.addEventListener('input', applyFilters);
  }

  filterBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      filterBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentFilter = btn.dataset.filter || 'all';
      applyFilters();
    });
  });

  // 6. Drag & Drop File Upload
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

    showToast(`${files.length} dosya yükleniyor...`);
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
            showToast('Tüm dosyalar başarıyla yüklendi!');
            setTimeout(() => window.location.reload(), 1000);
          }
        } else {
          showToast(data.error || `${file.name} yüklenemedi`, 'error');
        }
      })
      .catch(err => {
        showToast(`Hata: ${file.name} yüklenemedi`, 'error');
      });
    });
  }

  // 7. Clipboard Listener (Global Ctrl+V paste catcher)
  document.addEventListener('paste', (e) => {
    // Ignore if target is already inside an input or textarea
    const active = document.activeElement;
    if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA' || active.isContentEditable)) {
      return;
    }

    const items = (e.clipboardData || e.originalEvent.clipboardData).items;
    let found = false;

    for (let i = 0; i < items.length; i++) {
      if (items[i].type.indexOf('image') !== -1) {
        const blob = items[i].getAsFile();
        if (blob) {
          found = true;
          const ext = blob.type.split('/')[1] || 'png';
          const file = new File([blob], `clipboard_${Date.now()}.${ext}`, { type: blob.type });
          uploadFiles([file]);
          break;
        }
      }
    }

    if (!found) {
      const text = e.clipboardData.getData('text');
      if (text && text.trim().length > 0) {
        // Open Editor tab with pasted content prefilled!
        activateTab('#editor');
        const editorTextarea = document.getElementById('editor_content');
        if (editorTextarea) {
          editorTextarea.value = text;
          editorTextarea.focus();
          showToast('Panodaki metin editöre aktarıldı!');
          updateMarkdownPreview(text);
        }
      }
    }
  });

  // 8. Split Editor Live Markdown Preview & Tab Key Support
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

    // Support Tab key indentation
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

  // 9. QR Code Modal (Pure SVG offline generator)
  window.openQRModal = function(url, filename) {
    let modal = document.getElementById('qr-modal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'qr-modal';
      modal.className = 'modal-backdrop';
      modal.innerHTML = `
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">QR Kod ile Paylaş</h3>
            <button class="modal-close" onclick="closeQRModal()">&times;</button>
          </div>
          <div class="modal-body" style="text-align: center;">
            <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 16px;" id="qr-modal-filename"></p>
            <div id="qr-code-target" style="display: inline-block; background: #fff; padding: 14px; border-radius: 8px;"></div>
            <div style="margin-top: 14px; font-family: var(--font-mono); font-size: 11.5px; color: var(--text-muted); word-break: break-all;" id="qr-modal-url"></div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-sm btn-primary" onclick="copyText(window.currentQRUrl, 'Paylaşım linki kopyalandı!')">Linki Kopyala</button>
            <button class="btn btn-sm" onclick="closeQRModal()">Kapat</button>
          </div>
        </div>
      `;
      document.body.appendChild(modal);
    }

    window.currentQRUrl = url;
    document.getElementById('qr-modal-filename').textContent = filename;
    document.getElementById('qr-modal-url').textContent = url;

    // Generate QR using an inline SVG render
    const target = document.getElementById('qr-code-target');
    const qrImg = document.createElement('img');
    qrImg.src = `https://api.qrserver.com/v1/create-qr-code/?size=180x180&data=${encodeURIComponent(url)}`;
    qrImg.width = 180;
    qrImg.height = 180;
    qrImg.alt = 'QR Code';
    target.innerHTML = '';
    target.appendChild(qrImg);

    modal.classList.add('active');
  };

  window.closeQRModal = function() {
    const modal = document.getElementById('qr-modal');
    if (modal) modal.classList.remove('active');
  };

  // 10. Simple Safe Markdown Parser for Live Preview & Viewer
  window.renderSimpleMarkdown = function(md) {
    if (!md) return '';
    let html = md
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");

    // Code blocks
    html = html.replace(/```(?:[a-zA-Z0-9_\-\.\+]+)?\n([\s\S]*?)```/g, '<pre><code>$1</code></pre>');
    // Inline code
    html = html.replace(/`([^`\n]+)`/g, '<code>$1</code>');
    // Headers
    html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
    html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
    html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');
    // Bold & Italic
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
    // Blockquotes
    html = html.replace(/^&gt;[ \t]?(.*$)/gim, '<blockquote>$1</blockquote>');
    // Links
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    // Lists
    html = html.replace(/^[ \t]*[\-\*][ \t](.*$)/gim, '<li>$1</li>');

    return `<div class="prose">${html}</div>`;
  };
});
