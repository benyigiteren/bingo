// ==========================================================================
// Bingo - Istemci Etkilesim ve Arayuz Motoru (100% Turkce)
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

  // 2. Mobil Sidebar Aç/Kapa & Karartma Perdesi
  window.toggleSidebar = function() {
    const sidebar = document.getElementById('app-sidebar');
    const backdrop = document.getElementById('sidebar-backdrop');
    if (sidebar) sidebar.classList.toggle('open');
    if (backdrop) backdrop.classList.toggle('open');
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
    'files': document.getElementById('section-files'),
    'mcp': document.getElementById('section-mcp'),
    'users': document.getElementById('section-users'),
    'settings': document.getElementById('section-settings')
  };

  const tabTitles = {
    'share': 'Yeni Paylaşım',
    'files': 'İstatistikler',
    'mcp': 'API & MCP',
    'users': 'Kullanıcı Yönetimi',
    'settings': 'Sistem Ayarları'
  };

  window.activateTab = function(tabName) {
    if (!tabName) tabName = 'share';
    tabName = tabName.replace('#', '');

    sidebarItems.forEach(item => {
      if (item.dataset.tab === tabName) {
        item.classList.add('active');
      } else {
        item.classList.remove('active');
      }
    });

    // Panel görünürlüğü
    Object.keys(sections).forEach(key => {
      const panel = sections[key];
      if (panel) {
        if (key === tabName) {
          panel.classList.add('active');
        } else {
          panel.classList.remove('active');
        }
      }
    });

    // Üst bar başlığını dinamik güncelle
    const topbarSection = document.getElementById('topbar-active-section');
    if (topbarSection && tabTitles[tabName]) {
      topbarSection.textContent = tabTitles[tabName];
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
        const backdrop = document.getElementById('sidebar-backdrop');
        if (sidebar) sidebar.classList.remove('open');
        if (backdrop) backdrop.classList.remove('open');
      }
    });
  });

  window.addEventListener('popstate', () => {
    if (window.location.hash) {
      activateTab(window.location.hash);
    } else {
      activateTab('share');
    }
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

  // 7.1 API Key Maskeleme Aç/Kapat (Eye Toggle)
  window.toggleAPIKeyVisibility = function() {
    const input = document.getElementById('user-api-key-input');
    const icon = document.getElementById('eye-icon');
    if (!input) return;
    if (input.type === 'password') {
      input.type = 'text';
      if (icon) {
        icon.innerHTML = '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path><line x1="1" y1="1" x2="23" y2="23"></line>';
      }
    } else {
      input.type = 'password';
      if (icon) {
        icon.innerHTML = '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle>';
      }
    }
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

  // 9. Sürükle & Bırak, Dosya Seçimi ve Hazırlama Sahnesi
  const uploadZone = document.getElementById('upload-zone');
  const fileInput = document.getElementById('file-input');
  const fileStageCard = document.getElementById('file-stage-card');

  window.stagedFiles = [];

  function formatFileSize(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function getFileIconClass(filename) {
    const ext = (filename.split('.').pop() || '').toLowerCase();
    if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico'].includes(ext)) return 'ph-light ph-image';
    if (['pdf'].includes(ext)) return 'ph-light ph-file-pdf';
    if (['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)) return 'ph-light ph-file-archive';
    if (['go', 'py', 'js', 'ts', 'html', 'css', 'json', 'sql', 'sh', 'yaml', 'yml'].includes(ext)) return 'ph-light ph-file-code';
    if (['txt', 'md', 'log'].includes(ext)) return 'ph-light ph-file-text';
    if (['mp3', 'wav', 'ogg', 'flac'].includes(ext)) return 'ph-light ph-file-audio';
    if (['mp4', 'mkv', 'avi', 'mov', 'webm'].includes(ext)) return 'ph-light ph-file-video';
    return 'ph-light ph-file';
  }

  window.stageFiles = function(files) {
    if (!files || files.length === 0) return;
    window.stagedFiles = Array.from(files);

    if (typeof switchHeroMode === 'function') {
      switchHeroMode('upload');
    }

    if (uploadZone) uploadZone.style.display = 'none';
    if (fileStageCard) fileStageCard.style.display = 'flex';

    const stagedImg = document.getElementById('staged-img-preview');
    const stagedIcon = document.getElementById('staged-icon-preview');
    const origNameEl = document.getElementById('staged-orig-name');
    const sizeBadgeEl = document.getElementById('staged-file-size');
    const subtextEl = document.getElementById('staged-file-subtext');
    const filenameInput = document.getElementById('staged-filename-input');
    const filenameGroup = document.getElementById('staged-filename-group');
    const multiList = document.getElementById('staged-multi-list');
    const uploadBtnText = document.getElementById('btn-staged-upload-text');

    if (window.stagedFiles.length === 1) {
      const file = window.stagedFiles[0];
      if (origNameEl) origNameEl.textContent = file.name;
      if (sizeBadgeEl) sizeBadgeEl.textContent = formatFileSize(file.size);
      if (subtextEl) subtextEl.textContent = file.type || 'Dosya hazır · Ayarları düzenleyip yükleyebilirsiniz';
      if (filenameGroup) filenameGroup.style.display = 'block';
      if (filenameInput) {
        filenameInput.value = file.name;
        filenameInput.focus();
      }
      if (multiList) multiList.style.display = 'none';
      if (uploadBtnText) uploadBtnText.textContent = 'Yükle ve Paylaş';

      if (file.type && file.type.startsWith('image/')) {
        const reader = new FileReader();
        reader.onload = (e) => {
          if (stagedImg) {
            stagedImg.src = e.target.result;
            stagedImg.style.display = 'block';
          }
          if (stagedIcon) stagedIcon.style.display = 'none';
        };
        reader.readAsDataURL(file);
      } else {
        if (stagedImg) stagedImg.style.display = 'none';
        if (stagedIcon) {
          stagedIcon.className = getFileIconClass(file.name) + ' staged-file-icon';
          stagedIcon.style.display = 'block';
        }
      }
    } else {
      if (origNameEl) origNameEl.textContent = `${window.stagedFiles.length} dosya seçildi`;
      const totalBytes = window.stagedFiles.reduce((acc, f) => acc + f.size, 0);
      if (sizeBadgeEl) sizeBadgeEl.textContent = `Toplam ${formatFileSize(totalBytes)}`;
      if (subtextEl) subtextEl.textContent = 'Seçenekler (TTL, Şifre, Burn) tüm dosyalara uygulanacaktır';
      if (filenameGroup) filenameGroup.style.display = 'none';
      if (stagedImg) stagedImg.style.display = 'none';
      if (stagedIcon) {
        stagedIcon.className = 'ph-light ph-files staged-file-icon';
        stagedIcon.style.display = 'block';
      }
      if (multiList) {
        multiList.style.display = 'flex';
        multiList.innerHTML = window.stagedFiles.map(f => `
          <div class="staged-multi-item">
            <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${f.name}</span>
            <span style="color: var(--text-muted); font-family: var(--font-mono); font-size: 11px;">${formatFileSize(f.size)}</span>
          </div>
        `).join('');
      }
      if (uploadBtnText) uploadBtnText.textContent = `Tümünü Yükle (${window.stagedFiles.length})`;
    }
  };

  window.resetStagedFile = function() {
    window.stagedFiles = [];
    if (fileInput) fileInput.value = '';
    const filenameInput = document.getElementById('staged-filename-input');
    if (filenameInput) filenameInput.value = '';
    const passwordInput = document.getElementById('upload-password-input');
    if (passwordInput) passwordInput.value = '';
    const ttlSelect = document.getElementById('upload-ttl-select');
    if (ttlSelect) ttlSelect.value = 'forever';
    const burnCheckbox = document.getElementById('upload-burn-checkbox');
    if (burnCheckbox) burnCheckbox.checked = false;

    const accordionContent = document.getElementById('upload-accordion-content');
    const accordionBtn = document.getElementById('upload-accordion-toggle-btn');
    if (accordionContent) accordionContent.classList.remove('open');
    if (accordionBtn) accordionBtn.classList.remove('open');

    if (fileStageCard) fileStageCard.style.display = 'none';
    if (uploadZone) uploadZone.style.display = 'block';
  };

  window.toggleUploadAdvancedOptions = function() {
    const content = document.getElementById('upload-accordion-content');
    const btn = document.getElementById('upload-accordion-toggle-btn');
    if (content) content.classList.toggle('open');
    if (btn) btn.classList.toggle('open');
  };

  window.startStagedUpload = async function() {
    if (!window.stagedFiles || window.stagedFiles.length === 0) {
      showToast('Yüklenecek dosya bulunamadı', 'error');
      return;
    }

    const uploadBtn = document.getElementById('btn-staged-upload');
    const uploadBtnText = document.getElementById('btn-staged-upload-text');
    if (uploadBtn) uploadBtn.disabled = true;
    if (uploadBtnText) uploadBtnText.textContent = 'Yükleniyor...';

    const ttl = (document.getElementById('upload-ttl-select') || {}).value || 'forever';
    const password = ((document.getElementById('upload-password-input') || {}).value || '').trim();
    const isBurn = !!((document.getElementById('upload-burn-checkbox') || {}).checked);
    const customFilename = ((document.getElementById('staged-filename-input') || {}).value || '').trim();
    const csrfToken = (document.querySelector('input[name="csrf_token"]') || {}).value || '';

    showToast(`${window.stagedFiles.length} dosya yükleniyor...`);
    let completed = 0;
    let hasError = false;
    let lastUrl = '';

    for (let i = 0; i < window.stagedFiles.length; i++) {
      const file = window.stagedFiles[i];
      const formData = new FormData();
      formData.append('file', file);
      if (csrfToken) formData.append('csrf_token', csrfToken);
      if (ttl) formData.append('ttl', ttl);
      if (password) formData.append('password', password);
      if (isBurn) formData.append('is_burn', 'true');

      if (window.stagedFiles.length === 1 && customFilename) {
        formData.append('filename', customFilename);
      }

      try {
        const resp = await fetch('/dashboard/upload', {
          method: 'POST',
          body: formData
        });
        const data = await resp.json();
        if (resp.ok && data.success) {
          completed++;
          if (data.url) lastUrl = data.url;
        } else {
          hasError = true;
          showToast(data.error || `${file.name} yüklenemedi`, 'error');
        }
      } catch (err) {
        hasError = true;
        showToast(`Bağlantı hatası: ${file.name} yüklenemedi`, 'error');
      }
    }

    if (uploadBtn) uploadBtn.disabled = false;
    if (uploadBtnText) uploadBtnText.textContent = 'Yükle ve Paylaş';

    if (completed > 0 && !hasError) {
      showToast('Yükleme başarıyla tamamlandı!');
      setTimeout(() => {
        if (window.stagedFiles.length === 1 && lastUrl) {
          window.location.href = lastUrl;
        } else {
          window.location.reload();
        }
      }, 600);
    }
  };

  if (uploadZone && fileInput) {
    uploadZone.addEventListener('click', () => fileInput.click());

    fileInput.addEventListener('change', () => {
      if (fileInput.files.length > 0) {
        stageFiles(Array.from(fileInput.files));
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
      if (files && files.length > 0) {
        stageFiles(Array.from(files));
      }
    });
  }

  // Pencere genelinde sürükle bırak koruması
  window.addEventListener('dragover', (e) => e.preventDefault(), false);
  window.addEventListener('drop', (e) => {
    if (e.target !== uploadZone && (!uploadZone || !uploadZone.contains(e.target)) && e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      e.preventDefault();
      stageFiles(Array.from(e.dataTransfer.files));
    }
  }, false);

  // 10. Global Pano Yapıştırma Dinleyicisi (Ctrl + V)
  document.addEventListener('paste', (e) => {
    const active = document.activeElement;
    if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA' || active.isContentEditable)) {
      return;
    }

    const items = (e.clipboardData || e.originalEvent.clipboardData).items;
    let foundFile = false;

    for (let i = 0; i < items.length; i++) {
      if (items[i].kind === 'file') {
        const blob = items[i].getAsFile();
        if (blob) {
          foundFile = true;
          let filename = blob.name;
          if (!filename || filename === 'image.png' || filename === 'blob') {
            const ext = (blob.type.split('/')[1] || 'png').replace('jpeg', 'jpg');
            const now = new Date();
            const dateStr = now.toISOString().slice(0, 10).replace(/-/g, '') + '_' + now.toTimeString().slice(0, 8).replace(/:/g, '');
            filename = `ekran_goruntusu_${dateStr}.${ext}`;
          }
          const file = new File([blob], filename, { type: blob.type });
          stageFiles([file]);
          showToast('Görsel panodan aktarıldı, düzenleyip yükleyebilirsiniz.');
          break;
        }
      }
    }

    if (!foundFile) {
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

  // 14. Şifre Değiştirme Modalı ve İşlemleri
  window.openPasswordModal = function() {
    const modal = document.getElementById('password-modal');
    if (!modal) return;
    modal.style.display = 'flex';
    modal.classList.add('active');
    const currInput = document.getElementById('modal-current-password');
    if (currInput) setTimeout(() => currInput.focus(), 50);
  };

  window.closePasswordModal = function(e) {
    if (e && e.target && e.target !== e.currentTarget) return;
    const modal = document.getElementById('password-modal');
    if (!modal) return;
    modal.style.display = 'none';
    modal.classList.remove('active');
    const form = document.getElementById('change-password-modal-form');
    if (form) form.reset();
  };

  window.handlePasswordChange = async function(e) {
    e.preventDefault();
    const form = e.target;
    const currentPass = form.current_password.value;
    const newPass = form.new_password.value;
    const confirmPass = form.confirm_password.value;
    const csrfToken = form.csrf_token.value;

    if (newPass.length < 6) {
      showToast('Yeni şifre en az 6 karakter olmalıdır.', 'error');
      return;
    }

    if (newPass !== confirmPass) {
      showToast('Yeni şifreler birbiriyle eşleşmiyor.', 'error');
      return;
    }

    const submitBtn = document.getElementById('btn-modal-save-password');
    if (submitBtn) submitBtn.disabled = true;

    try {
      const formData = new URLSearchParams();
      formData.append('csrf_token', csrfToken);
      formData.append('current_password', currentPass);
      formData.append('new_password', newPass);
      formData.append('confirm_password', confirmPass);

      const resp = await fetch('/dashboard/user/change-password', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
          'X-Requested-With': 'XMLHttpRequest'
        },
        body: formData.toString()
      });

      const data = await resp.json();
      if (resp.ok && data.success) {
        showToast(data.message || 'Şifreniz başarıyla değiştirildi.');
        closePasswordModal();
      } else {
        showToast(data.error || 'Şifre değiştirilemedi.', 'error');
      }
    } catch (err) {
      showToast('Bir bağlantı hatası oluştu.', 'error');
    } finally {
      if (submitBtn) submitBtn.disabled = false;
    }
  };

  window.handleSettingsPasswordSubmit = function(e) {
    const form = e.target;
    const newPass = form.new_password.value;
    const confirmPass = form.confirm_password.value;
    if (newPass.length < 6) {
      showToast('Yeni şifre en az 6 karakter olmalıdır.', 'error');
      e.preventDefault();
      return false;
    }
    if (newPass !== confirmPass) {
      showToast('Yeni şifreler birbiriyle eşleşmiyor.', 'error');
      e.preventDefault();
      return false;
    }
    return true;
  };

  // 12. MCP Format Değiştirici (Gemini / Claude / Stdio / CLI)
  window.switchMCPFormat = function(type) {
    const rawElem = document.getElementById('raw-mcp-' + type);
    const preElem = document.getElementById('mcp-json-config');
    if (rawElem && preElem) {
      preElem.textContent = rawElem.textContent.trim();
    }
    document.querySelectorAll('.mcp-client-tab').forEach(tab => {
      if (tab.getAttribute('data-client') === type) {
        tab.classList.add('active');
      } else {
        tab.classList.remove('active');
      }
    });
  };
});

// Güvenli mini Markdown işleyici (XSS korumalı: önce HTML kaçış, sonra sınırlı biçim).
// viewer.html içindeki #markdown-viewer alanını doldurur. Yalnızca http/https
// bağlantılarına izin verilir; javascript:/data: şemaları düz metin olarak kalır.
window.renderSimpleMarkdown = function(src) {
  if (src == null) return '';
  var text = String(src);
  // Çok büyük girdilerde tarayıcıyı kilitlememek için üst sınır.
  if (text.length > 500000) text = text.slice(0, 500000);

  function esc(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function inline(s) {
    var e = esc(s);
    e = e.replace(/`([^`\n]+)`/g, '<code>$1</code>');
    e = e.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    e = e.replace(/(^|[\s(])\*([^*\n]+)\*/g, '$1<em>$2</em>');
    e = e.replace(/\[([^\]]+)\]\((https?:\/\/[^)\s"']+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
    return e;
  }

  var lines = text.split('\n');
  var html = '';
  var inFence = false;
  var fenceBuf = [];
  var inList = false;

  function closeList() {
    if (inList) { html += '</ul>'; inList = false; }
  }

  for (var i = 0; i < lines.length; i++) {
    var line = lines[i];
    if (/^```/.test(line)) {
      if (inFence) {
        html += '<pre><code>' + esc(fenceBuf.join('\n')) + '</code></pre>';
        fenceBuf = [];
        inFence = false;
      } else {
        closeList();
        inFence = true;
      }
      continue;
    }
    if (inFence) { fenceBuf.push(line); continue; }

    var h = line.match(/^(#{1,6})\s+(.*)$/);
    if (h) {
      closeList();
      var lvl = h[1].length;
      html += '<h' + lvl + '>' + inline(h[2]) + '</h' + lvl + '>';
      continue;
    }
    var li = line.match(/^\s*[-*]\s+(.*)$/);
    if (li) {
      if (!inList) { html += '<ul>'; inList = true; }
      html += '<li>' + inline(li[1]) + '</li>';
      continue;
    }
    if (/^\s*$/.test(line)) { closeList(); continue; }
    closeList();
    html += '<p>' + inline(line) + '</p>';
  }
  if (inFence) html += '<pre><code>' + esc(fenceBuf.join('\n')) + '</code></pre>';
  closeList();
  return html;
};
