// ==========================================================================
// Bingo — İstemci Etkileşim ve Arayüz Motoru (100% Türkçe)
// ==========================================================================

document.addEventListener('DOMContentLoaded', () => {
  // 0. Otomatik Doldurma (Autofill) Önleyici Güvenlik
  // Tarayıcıların kasa/oluşturma alanlarına yönetici şifrelerini istenmeyen şekilde basmasını engeller
  setTimeout(() => {
    document.querySelectorAll('input[type="password"]').forEach(input => {
      if (input.name === 'password' && !input.closest('#login-form')) {
        input.value = '';
      }
    });
  }, 100);

  // 1. Tema Yönetimi (Açık / Koyu Tema - Varsayılan: Açık/Koyu Tercihi)
  const savedTheme = localStorage.getItem('bingo-theme') || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  document.documentElement.setAttribute('data-theme', savedTheme);
  updateThemeIcon(savedTheme);

  window.toggleTheme = function() {
    const current = document.documentElement.getAttribute('data-theme') || 'light';
    const next = current === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('bingo-theme', next);
    updateThemeIcon(next);
  };

  function updateThemeIcon(theme) {
    const icon = document.getElementById('theme-icon');
    if (!icon) return;
    if (theme === 'dark') {
      // Güneş ikonu
      icon.innerHTML = `<circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>`;
    } else {
      // Ay ikonu
      icon.innerHTML = `<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>`;
    }
  }

  // 2. Mobil Sidebar Aç/Kapa
  window.toggleSidebar = function() {
    const sidebar = document.getElementById('app-sidebar');
    if (sidebar) sidebar.classList.toggle('open');
  };

  // 3. Hero Paylaşım Modu Değiştirici (Dosya Yükle / Kod Editörü)
  window.switchHeroMode = function(mode) {
    const uploadPane = document.getElementById('hero-upload-pane');
    const editorPane = document.getElementById('hero-editor-pane');
    const btnUpload = document.getElementById('tab-btn-upload');
    const btnEditor = document.getElementById('tab-btn-editor');

    if (mode === 'editor') {
      if (uploadPane) uploadPane.style.display = 'none';
      if (editorPane) editorPane.classList.add('active');
      if (btnUpload) btnUpload.classList.remove('active');
      if (btnEditor) btnEditor.classList.add('active');
      const input = document.getElementById('editor_filename');
      if (input) input.focus();
    } else {
      if (uploadPane) uploadPane.style.display = 'block';
      if (editorPane) editorPane.classList.remove('active');
      if (btnUpload) btnUpload.classList.add('active');
      if (btnEditor) btnEditor.classList.remove('active');
    }
  };

  // 3.1 Canlı Karakter ve Satır Sayacı
  const editorContentElem = document.getElementById('editor_content');
  const charCounterElem = document.getElementById('char-counter');
  if (editorContentElem && charCounterElem) {
    editorContentElem.addEventListener('input', () => {
      const val = editorContentElem.value;
      const chars = val.length;
      const lines = val ? val.split('\n').length : 0;
      charCounterElem.textContent = `${chars} karakter · ${lines} satır`;
    });
  }

  // 3.2 Akıllı İki Yönlü Dosya Uzantısı Senkronizasyonu
  const filenameInput = document.getElementById('editor_filename');
  const langSelect = document.getElementById('editor_language');

  if (filenameInput && langSelect) {
    // Dropdown değiştiğinde dosya adının uzantısını otomatik tamamla / güncelle
    langSelect.addEventListener('change', () => {
      const selectedExt = langSelect.value;
      if (!selectedExt) return;

      let currentName = filenameInput.value.trim();
      if (!currentName) {
        filenameInput.value = 'yeni_kod' + selectedExt;
        return;
      }

      const dotIdx = currentName.lastIndexOf('.');
      if (dotIdx !== -1) {
        currentName = currentName.substring(0, dotIdx);
      }
      filenameInput.value = currentName + selectedExt;
    });

    // Dosya adı alanına uzantı yazıldığında dropdown'ı otomatik eşleştir
    filenameInput.addEventListener('input', () => {
      const val = filenameInput.value.trim();
      const dotIdx = val.lastIndexOf('.');
      if (dotIdx !== -1) {
        const typedExt = val.substring(dotIdx).toLowerCase();
        for (let i = 0; i < langSelect.options.length; i++) {
          if (langSelect.options[i].value.toLowerCase() === typedExt) {
            langSelect.selectedIndex = i;
            break;
          }
        }
      }
    });
  }

  // 4. Akordiyon (Gelişmiş Seçenekler)
  window.toggleAdvancedOptions = function() {
    const content = document.getElementById('accordion-content');
    const btn = document.getElementById('accordion-toggle-btn');
    if (content) content.classList.toggle('open');
    if (btn) btn.classList.toggle('open');
  };

  // 5. Sidebar Sekme Değişimi
  const sidebarItems = document.querySelectorAll('.sidebar-nav-item');
  const sections = {
    'share': document.getElementById('section-share'),
    'files': document.getElementById('section-share'),
    'mcp': document.getElementById('section-mcp'),
    'users': document.getElementById('section-users')
  };

  window.activateTab = function(tabName) {
    if (!tabName) tabName = 'share';
    tabName = tabName.replace('#', '');

    sidebarItems.forEach(item => {
      if (item.dataset.tab === tabName || (tabName === 'files' && item.dataset.tab === 'files')) {
        item.classList.add('active');
      } else {
        item.classList.remove('active');
      }
    });

    // Panel görünürlüğü
    Object.keys(sections).forEach(key => {
      const panel = sections[key];
      if (panel) {
        if (key === tabName || (tabName === 'files' && key === 'share')) {
          panel.classList.add('active');
        } else if (key !== 'files') {
          panel.classList.remove('active');
        }
      }
    });

    if (tabName === 'files') {
      const tableElem = document.getElementById('files-table-container');
      if (tableElem) {
        tableElem.scrollIntoView({ behavior: 'smooth' });
      }
    }
  };

  sidebarItems.forEach(item => {
    item.addEventListener('click', (e) => {
      const href = item.getAttribute('href');
      if (href && href.startsWith('/dashboard#')) {
        e.preventDefault();
        const tab = href.split('#')[1];
        history.pushState(null, '', '#' + tab);
        activateTab(tab);
        const sidebar = document.getElementById('app-sidebar');
        if (sidebar) sidebar.classList.remove('open');
      }
    });
  });

  if (window.location.hash) {
    activateTab(window.location.hash);
  }

  // 6. Türkçe Toast Bildirim Sistemi
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

  // 7. Panoya Kopyalama Aracı
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

  // 8. Tablo Arama & Kategori Filtreleme
  const searchInput = document.getElementById('file-search-input');
  const filterPills = document.querySelectorAll('.filter-pill');
  let currentFilter = 'all';

  function applyFilters() {
    const query = searchInput ? searchInput.value.toLowerCase().trim() : '';
    const rows = document.querySelectorAll('.paste-row');

    function matchesCategory(filename, cat) {
      if (cat === 'all') return true;
      const lower = filename.toLowerCase().trim();
      const dotIdx = lower.lastIndexOf('.');
      const ext = dotIdx !== -1 ? lower.substring(dotIdx) : '';

      if (cat === 'code') {
        const codeExts = ['.go', '.py', '.js', '.ts', '.jsx', '.tsx', '.rs', '.c', '.cpp', '.h', '.hpp', '.java', '.html', '.css', '.sh', '.bash', '.sql', '.yaml', '.yml', '.json', '.php', '.rb', '.lua', '.vue', '.svelte'];
        return codeExts.includes(ext) || lower === 'dockerfile' || lower === 'makefile' || lower.startsWith('docker-compose');
      }
      if (cat === 'text') {
        const textExts = ['.md', '.txt', '.log', '.env', '.ini', '.conf', '.csv', '.tsv', '.xml', '.pdf', '.doc', '.docx'];
        return textExts.includes(ext) || dotIdx === -1;
      }
      if (cat === 'images') {
        const imgExts = ['.png', '.jpg', '.jpeg', '.webp', '.gif', '.svg', '.bmp', '.ico', '.tiff', '.avif'];
        return imgExts.includes(ext);
      }
      return true;
    }

    rows.forEach(row => {
      const filename = row.dataset.filename || '';
      const textMatch = !query || filename.toLowerCase().includes(query);
      const catMatch = matchesCategory(filename, currentFilter);
      row.style.display = (textMatch && catMatch) ? '' : 'none';
    });
  }

  if (searchInput) {
    searchInput.addEventListener('input', applyFilters);
  }

  filterPills.forEach(pill => {
    pill.addEventListener('click', () => {
      filterPills.forEach(p => p.classList.remove('active'));
      pill.classList.add('active');
      currentFilter = pill.dataset.filter || 'all';
      applyFilters();
    });
  });

  // 9. Sürükle & Bırak ile Dosya Yükleme
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
            showToast('Yükleme başarıyla tamamlandı!');
            setTimeout(() => window.location.reload(), 600);
          }
        } else {
          showToast(data.error || `${file.name} yüklenemedi`, 'error');
        }
      })
      .catch(() => {
        showToast(`Hata: ${file.name} yüklenemedi`, 'error');
      });
    });
  }

  // 10. Global Pano Yapıştırma Dinleyicisi (Ctrl + V)
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
          const file = new File([blob], `ekran_goruntusu_${Date.now()}.${ext}`, { type: blob.type });
          uploadFiles([file]);
          break;
        }
      }
    }

    if (!foundImage) {
      const text = e.clipboardData.getData('text');
      if (text && text.trim().length > 0) {
        switchHeroMode('editor');
        const editorTextarea = document.getElementById('editor_content');
        if (editorTextarea) {
          editorTextarea.value = text;
          editorTextarea.focus();
          showToast('Metin editöre aktarıldı');
        }
      }
    }
  });

  // 11. QR Kod Paylaşım Modalı (Türkçe)
  window.openQRModal = function(url, filename) {
    let modal = document.getElementById('qr-modal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'qr-modal';
      modal.className = 'modal-backdrop';
      modal.innerHTML = `
        <div class="modal-box">
          <div class="modal-header">
            <span>QR Kod ile Paylaş</span>
            <button onclick="closeQRModal()" style="background:none; border:none; color:var(--text-muted); cursor:pointer; font-size:18px;">&times;</button>
          </div>
          <div style="text-align: center;">
            <div style="font-family: var(--font-mono); font-size: 13px; color: var(--text-secondary); margin-bottom: 14px;" id="qr-modal-filename"></div>
            <div id="qr-code-target" style="display: inline-block; background: #ffffff; padding: 12px; border-radius: var(--radius-md); border: 1px solid var(--border-card);"></div>
            <div style="margin-top: 14px; font-family: var(--font-mono); font-size: 12px; color: var(--text-muted); word-break: break-all;" id="qr-modal-url"></div>
          </div>
          <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; border-top: 1px solid var(--border-card); padding-top: 14px;">
            <button class="btn-action" onclick="copyText(window.currentQRUrl, 'Bağlantı kopyalandı!')">Bağlantıyı Kopyala</button>
            <button class="btn-action" onclick="closeQRModal()">Kapat</button>
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
    qrImg.src = `https://api.qrserver.com/v1/create-qr-code/?size=180x180&data=${encodeURIComponent(url)}`;
    qrImg.width = 180;
    qrImg.height = 180;
    qrImg.alt = 'QR Kodu';
    target.innerHTML = '';
    target.appendChild(qrImg);

    modal.classList.add('active');
  };

  window.closeQRModal = function() {
    const modal = document.getElementById('qr-modal');
    if (modal) modal.classList.remove('active');
  };
});
