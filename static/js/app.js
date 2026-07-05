// Bingo - Ön Yüz Etkileşim ve Yardımcı Fonksiyonlar

document.addEventListener('DOMContentLoaded', () => {
  // Toast Bildirim Sistemi
  window.showToast = function(message, type = 'success') {
    let container = document.getElementById('toast-container');
    if (!container) {
      container = document.createElement('div');
      container.id = 'toast-container';
      document.body.appendChild(container);
    }

    const toast = document.createElement('div');
    toast.className = `toast ${type === 'error' ? 'alert-danger' : 'alert-success'}`;
    
    // Basit minimalist bildirim ikonu (X veya Tik)
    const icon = type === 'error' 
      ? `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg>`
      : `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>`;

    toast.innerHTML = `${icon} <span>${message}</span>`;
    container.appendChild(toast);

    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transform = 'translateY(10px)';
      toast.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
      setTimeout(() => toast.remove(), 300);
    }, 3000);
  };

  // Panoya Kopyalama Yardımcısı
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

  // Sürükle Bırak Dosya Yükleme İşlemleri
  const uploadZone = document.getElementById('upload-zone');
  const fileInput = document.getElementById('file-input');

  if (uploadZone && fileInput) {
    uploadZone.addEventListener('click', () => fileInput.click());

    fileInput.addEventListener('change', () => {
      if (fileInput.files.length > 0) {
        handleUpload(fileInput.files[0]);
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
        handleUpload(files[0]);
      }
    });
  }

  function handleUpload(file) {
    const formData = new FormData();
    formData.append('file', file);

    // CSRF Token değerini al ve form verisine ekle
    const csrfInput = document.querySelector('input[name="csrf_token"]');
    if (csrfInput) {
      formData.append('csrf_token', csrfInput.value);
    }

    showToast('Dosya yükleniyor...', 'success');

    fetch('/dashboard/upload', {
      method: 'POST',
      body: formData
    })
    .then(response => {
      if (!response.ok) {
        return response.json().then(data => {
          throw new Error(data.error || 'Yükleme başarısız oldu');
        });
      }
      return response.json();
    })
    .then(data => {
      if (data.success) {
        showToast('Yükleme başarıyla tamamlandı!');
        setTimeout(() => {
          window.location.reload();
        }, 1200);
      } else {
        showToast(data.error || 'Yükleme başarısız', 'error');
      }
    })
    .catch(error => {
      showToast(error.message || 'Yükleme sırasında bir hata oluştu', 'error');
    });
  }

  // API Anahtarını Yenileme (AJAX ile)
  const regenBtn = document.getElementById('regenerate-api-key-btn');
  if (regenBtn) {
    regenBtn.addEventListener('click', (e) => {
      e.preventDefault();
      if (!confirm('API anahtarınızı yenilemek istediğinizden emin misiniz? Mevcut anahtarı kullanan eski betikler ve entegrasyonlar çalışmayı durduracaktır.')) {
        return;
      }

      const form = regenBtn.closest('form');
      const formData = new FormData(form);

      fetch('/dashboard/users/regenerate-key', {
        method: 'POST',
        headers: {
          'X-Requested-With': 'XMLHttpRequest'
        },
        body: formData
      })
      .then(response => response.json())
      .then(data => {
        if (data.success) {
          const keyDisplay = document.getElementById('api-key-display');
          if (keyDisplay) {
            keyDisplay.textContent = data.api_key;
          }
          showToast('API Anahtarı başarıyla yenilendi');
        } else {
          showToast('Anahtar yenilenemedi', 'error');
        }
      })
      .catch(() => {
        showToast('API Anahtarı yenilenirken bir hata oluştu', 'error');
      });
    });
  }

  // URL hash değerine göre sekmeleri yönetme
  const hash = window.location.hash || '#files';
  activateTab(hash);

  window.addEventListener('hashchange', () => {
    const currentHash = window.location.hash || '#files';
    activateTab(currentHash);
  });

  function activateTab(targetHash) {
    const tabLinks = document.querySelectorAll('.tab-link');
    const tabSections = document.querySelectorAll('.tab-section');
    
    // Sadece tanımlı hash değerlerini eşleştir
    const validHashes = ['#files', '#editor', '#users'];
    if (!validHashes.includes(targetHash)) return;

    tabLinks.forEach(link => {
      if (link.getAttribute('href') === targetHash) {
        link.classList.add('active');
      } else {
        link.classList.remove('active');
      }
    });

    tabSections.forEach(section => {
      if ('#' + section.id === targetHash) {
        section.style.display = 'block';
      } else {
        section.style.display = 'none';
      }
    });
  }
});
