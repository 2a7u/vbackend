    // =========================================================================
    // GLOBAL STATE & UTILITIES
    // =========================================================================
    let authToken = localStorage.getItem('venus_admin_token') || '';
    let currentUser = JSON.parse(localStorage.getItem('venus_admin_user') || 'null');
    let allUsers = [];
    let currentFilter = 'all';
    let currentTab = 'stats';

    function formatIQD(amount) {
      if (amount === undefined || amount === null) return '0 د.ع';
      return parseInt(amount, 10).toLocaleString('en-US') + ' د.ع';
    }

    function formatBaghdad(dateStr) {
      if (!dateStr) return '—';
      try {
        const d = new Date(dateStr);
        return d.toLocaleString('ar-IQ', { timeZone: 'Asia/Baghdad', hour12: true });
      } catch (e) {
        return dateStr;
      }
    }

    function formatDate(dateStr) {
      if (!dateStr) return '—';
      try {
        const d = new Date(dateStr);
        return d.toLocaleDateString('ar-IQ', { timeZone: 'Asia/Baghdad' });
      } catch (e) {
        return dateStr;
      }
    }

    function escapeHtml(str) {
      if (!str) return '';
      return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
    }

    function showToast(message, type = 'success') {
      const toast = document.getElementById('toast');
      const text = document.getElementById('toast-text');
      const icon = document.getElementById('toast-icon');
      if (!toast) return;

      text.innerText = message;
      if (type === 'error') {
        icon.className = 'fa-solid fa-circle-xmark text-rose-400';
      } else {
        icon.className = 'fa-solid fa-circle-check text-emerald-400';
      }

      toast.classList.remove('opacity-0', 'translate-y-20', 'pointer-events-none');
      setTimeout(() => {
        toast.classList.add('opacity-0', 'translate-y-20', 'pointer-events-none');
      }, 3500);
    }

    async function apiFetch(url, options = {}) {
      options.headers = options.headers || {};
      if (authToken) {
        options.headers['Authorization'] = 'Bearer ' + authToken;
      }
      if (options.body && typeof options.body === 'string' && !options.headers['Content-Type']) {
        options.headers['Content-Type'] = 'application/json';
      }
      if (!options.headers['Idempotency-Key'] && (options.method === 'POST' || options.method === 'PUT')) {
        options.headers['Idempotency-Key'] = 'key_' + Date.now() + '_' + Math.random().toString(36).substring(2, 9);
      }

      const res = await fetch(url, options);
      if (res.status === 401) {
        handleLogout();
        throw new Error('انتهت الجلسة، يرجى إعادة تسجيل الدخول');
      }
      return res;
    }

    function openModal(id) {
      const el = document.getElementById(id);
      if (el) el.classList.remove('hidden');
    }

    function closeModal(id) {
      const el = document.getElementById(id);
      if (el) el.classList.add('hidden');
    }

    function toggleMobileSidebar() {
      const sidebar = document.getElementById('mobile-sidebar');
      const backdrop = document.getElementById('mobile-sidebar-backdrop');
      if (!sidebar) return;
      if (sidebar.classList.contains('translate-x-full')) {
        sidebar.classList.remove('translate-x-full');
        backdrop.classList.remove('hidden');
      } else {
        sidebar.classList.add('translate-x-full');
        backdrop.classList.add('hidden');
      }
    }

    function updateClock() {
      const clockEl = document.getElementById('baghdad-clock');
      if (!clockEl) return;
      const now = new Date();
      clockEl.innerText = now.toLocaleTimeString('ar-IQ', { timeZone: 'Asia/Baghdad' }) + ' بتوقيت بغداد';
    }
    setInterval(updateClock, 1000);
    updateClock();

    // =========================================================================
    // AUTHENTICATION & APP LIFECYCLE
    // =========================================================================
    function showLogin() {
      document.getElementById('view-login').classList.remove('hidden');
      document.getElementById('view-app').classList.add('hidden');
    }

    function showApp() {
      document.getElementById('view-login').classList.add('hidden');
      document.getElementById('view-app').classList.remove('hidden');
      if (currentUser && document.getElementById('admin-name')) {
        document.getElementById('admin-name').innerText = currentUser.full_name || currentUser.email;
      }
      switchTab('stats');
      initConsoleWebSocket();
      checkRadarAlerts();
    }

    async function handleLogin(e) {
      e.preventDefault();
      const errBox = document.getElementById('login-error');
      errBox.classList.add('hidden');
      const email = document.getElementById('login-email').value.trim();
      const password = document.getElementById('login-password').value;

      try {
        const res = await fetch('/api/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ email, password })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تسجيل الدخول');
        if (data.user && data.user.role !== 'admin' && data.user.role !== 'platform_admin' && data.user.role !== 'superadmin') {
          throw new Error('هذا الحساب غير مصرح له بالدخول للوحة الإدارة');
        }

        authToken = data.token;
        currentUser = data.user;
        localStorage.setItem('venus_admin_token', authToken);
        localStorage.setItem('venus_admin_user', JSON.stringify(currentUser));
        showApp();
      } catch (err) {
        errBox.innerText = err.message;
        errBox.classList.remove('hidden');
      }
    }

    function handleLogout() {
      authToken = '';
      currentUser = null;
      localStorage.removeItem('venus_admin_token');
      localStorage.removeItem('venus_admin_user');
      showLogin();
    }

    if (authToken && currentUser) {
      showApp();
    } else {
      showLogin();
    }

    // =========================================================================
    // TAB SWITCHER
    // =========================================================================
    function switchTab(tabId) {
      currentTab = tabId;
      document.querySelectorAll('.tab-pane').forEach(el => el.classList.add('hidden'));
      const activePane = document.getElementById('tab-content-' + tabId);
      if (activePane) activePane.classList.remove('hidden');

      // Update sidebar styles
      document.querySelectorAll('.nav-tab-btn').forEach(btn => {
        btn.classList.remove('bg-brand', 'text-white', 'shadow-sm');
        btn.classList.add('text-slate-600', 'hover:bg-slate-50');
        const icon = btn.querySelector('i');
        if (icon) {
          icon.classList.add('text-slate-400');
          icon.classList.remove('text-white');
        }
      });
      const activeBtn = document.getElementById('nav-' + tabId);
      if (activeBtn) {
        activeBtn.classList.remove('text-slate-600', 'hover:bg-slate-50');
        activeBtn.classList.add('bg-brand', 'text-white', 'shadow-sm');
        const icon = activeBtn.querySelector('i');
        if (icon) {
          icon.classList.remove('text-slate-400');
          icon.classList.add('text-white');
        }
      }

      // Update Top Title
      const titleEl = document.getElementById('current-tab-title');
      const titles = {
        stats: 'الرئيسية',
        users: 'المشتركين',
        activation: 'الباقات',
        wallet: 'المحافظ',
        cards: 'البطاقات',
        audit: 'سجل التدقيق',
        ledger: 'السجل المالي',
        fraud: 'الحماية',
        reconcile: 'التسويات',
        debts: 'الائتمان',
        close: 'الإغلاق المالي',
        pricing: 'الأسعار',
        radar: 'مؤشر النفاد',
        coupons: 'الخصومات',
        appconfig: 'إعدادات المنصة',
        deviceradar: 'الأجهزة المتصلة',
        security: 'الرقابة والأمان',
        bi: 'التقارير'
      };
      if (titleEl && titles[tabId]) titleEl.innerText = titles[tabId];

      // Tab Data Loader Dispatches
      if (tabId === 'stats') loadStats();
      if (tabId === 'users') loadUsers();
      if (tabId === 'activation') loadActivationCodes();
      if (tabId === 'wallet') loadZainCashTransactions();
      if (tabId === 'cards') loadDigitalCards();
      if (tabId === 'audit') loadAuditLogs();
      if (tabId === 'ledger') { loadLedgerEntries(); loadIntegritySummary(); }
      if (tabId === 'fraud') loadFraudData();
      if (tabId === 'reconcile') { loadReconciliationData(); loadReconciliationReports(); }
      if (tabId === 'debts') loadAgentDebts();
      if (tabId === 'close') { loadFinancialCloses(); populateStatementUsers(); }
      if (tabId === 'pricing') { loadPricingData(); loadHierarchyData(); }
      if (tabId === 'radar') loadRadarData();
      if (tabId === 'coupons') loadCouponsData();
      if (tabId === 'appconfig') loadAppConfigData();
      if (tabId === 'deviceradar') loadDeviceRadarData();
      if (tabId === 'security') loadSecurityData();
      if (tabId === 'bi') loadBIData();
    }

    // =========================================================================
    // CORE TAB 1: STATS
    // =========================================================================
    async function loadStats() {
      try {
        const res = await apiFetch('/api/admin/stats');
        const data = await res.json();
        if (res.ok) {
          document.getElementById('stat-total-users').innerText = (data.total_users || 0).toLocaleString();
          document.getElementById('stat-active-subs').innerText = (data.active_subscriptions || 0).toLocaleString();
          document.getElementById('stat-total-balance').innerText = formatIQD(data.total_wallet_balance || 0);
          document.getElementById('stat-zaincash-today').innerText = formatIQD(data.zaincash_volume_today || 0);
        }
      } catch (err) {
        console.error('Stats error:', err);
      }
    }

    // =========================================================================
    // CORE TAB 2: SUBSCRIBERS & POS
    // =========================================================================
    async function loadUsers() {
      try {
        const res = await apiFetch('/api/admin/users');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          allUsers = data;
          renderUsersTable();
        }
      } catch (err) {
        console.error('Users load error:', err);
      }
    }

    function setUserFilter(filter) {
      currentFilter = filter;
      document.querySelectorAll('.user-filter-btn').forEach(btn => {
        if (btn.getAttribute('data-filter') === filter) {
          btn.className = 'user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold bg-brand text-white shadow-sm';
        } else {
          btn.className = 'user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold text-slate-600 hover:bg-slate-50';
        }
      });
      renderUsersTable();
    }

    function renderUsersTable() {
      const tbody = document.getElementById('users-table-body');
      if (!tbody) return;
      const q = (document.getElementById('user-search').value || '').toLowerCase();

      const filtered = allUsers.filter(u => {
        const matchesQuery = !q || (u.email && u.email.toLowerCase().includes(q)) ||
          (u.full_name && u.full_name.toLowerCase().includes(q)) ||
          (u.store_name && u.store_name.toLowerCase().includes(q)) ||
          (u.phone && u.phone.includes(q));

        if (!matchesQuery) return false;
        if (currentFilter === 'all') return true;
        if (currentFilter === 'pending') return !u.is_approved;
        return u.role === currentFilter;
      });

      if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="py-8 text-center text-slate-400 font-bold">لا يوجد مشتركون مطابقون للبحث</td></tr>';
        return;
      }

      tbody.innerHTML = filtered.map(u => {
        const roleBadges = {
          agent: '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-amber-50 text-amber-700">وكيل معتمد</span>',
          pos: '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-purple-50 text-purple-700">نقطة بيع</span>',
          subscriber: '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand">مشترك</span>',
          admin: '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-900 text-white">مدير</span>'
        };
        const subBadge = u.subscription_status 
          ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">نشط</span>'
          : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-500">منتهي</span>';

        return `
          <tr class="hover:bg-slate-50/80 transition">
            <td class="py-3 px-3">
              <div class="font-bold text-slate-800">${escapeHtml(u.store_name || u.full_name || u.email)}</div>
              <div class="text-[10px] text-slate-400 font-mono">${escapeHtml(u.email)}</div>
            </td>
            <td class="py-3 px-3 font-mono font-bold text-slate-600">${escapeHtml(u.phone || '—')}</td>
            <td class="py-3 px-3">${roleBadges[u.role] || u.role}</td>
            <td class="py-3 px-3 font-black text-emerald-600">${formatIQD(u.wallet_balance || 0)}</td>
            <td class="py-3 px-3">${subBadge}</td>
            <td class="py-3 px-3 text-[11px] text-slate-500 font-mono">${u.is_device_locked ? '<i class="fa-solid fa-lock text-amber-500 mr-1"></i>مقفل' : 'متاح'}</td>
            <td class="py-3 px-3 text-[11px] text-slate-500">${formatDate(u.created_at)}</td>
            <td class="py-3 px-3 text-center">
              <div class="flex items-center justify-center gap-1">
                <button onclick="openSubscriber360(${u.id})" title="ملف المشترك" class="p-1.5 rounded-lg text-blue-600 hover:bg-blue-50 transition"><i class="fa-solid fa-crosshairs"></i></button>
                <button onclick="startImpersonation(${u.id}, '${escapeHtml(u.full_name || u.email)}')" title="معاينة الحساب" class="p-1.5 rounded-lg text-teal-600 hover:bg-teal-50 transition"><i class="fa-solid fa-user-secret"></i></button>
                <button onclick="openDeviceControls(${u.id}, '${escapeHtml(u.email)}', ${u.is_device_locked || false}, '${escapeHtml(u.device_id || '')}')" title="أجهزة وجلسات المشترك" class="p-1.5 rounded-lg text-slate-600 hover:bg-slate-100 transition"><i class="fa-solid fa-mobile-screen-button"></i></button>
                <button onclick="openWalletAdjustModal(${u.id})" title="شحن الرصيد" class="p-1.5 rounded-lg text-emerald-600 hover:bg-emerald-50 transition"><i class="fa-solid fa-wallet"></i></button>
                <button onclick="openEditUserModal(${u.id})" title="تعديل البيانات" class="p-1.5 rounded-lg text-brand hover:bg-brand-light transition"><i class="fa-solid fa-pen-to-square"></i></button>
                <button onclick="deleteUserPrompt(${u.id}, '${escapeHtml(u.full_name || u.email)}')" title="حذف المشترك" class="p-1.5 rounded-lg text-rose-500 hover:bg-rose-50 transition"><i class="fa-solid fa-trash-can"></i></button>
              </div>
            </td>
          </tr>
        `;
      }).join('');
    }

    function openAddUserModal() {
      document.getElementById('add-user-name').value = '';
      document.getElementById('add-user-store').value = '';
      document.getElementById('add-user-email').value = '';
      document.getElementById('add-user-phone').value = '';
      document.getElementById('add-user-password').value = '';
      openModal('modal-add-user');
    }

    async function handleCreateUser(e) {
      e.preventDefault();
      const payload = {
        full_name: document.getElementById('add-user-name').value.trim(),
        store_name: document.getElementById('add-user-store').value.trim(),
        email: document.getElementById('add-user-email').value.trim(),
        phone: document.getElementById('add-user-phone').value.trim(),
        password: document.getElementById('add-user-password').value,
        role: document.getElementById('add-user-role').value
      };

      try {
        const res = await apiFetch('/api/admin/users', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل إنشاء المشترك');

        closeModal('modal-add-user');
        showToast('تمت إضافة المشترك بنجاح');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function openEditUserModal(id) {
      const u = allUsers.find(x => x.id === id);
      if (!u) return;
      document.getElementById('edit-user-id').value = u.id;
      document.getElementById('edit-user-name').value = u.full_name || '';
      document.getElementById('edit-user-store').value = u.store_name || '';
      document.getElementById('edit-user-phone').value = u.phone || '';
      document.getElementById('edit-user-role').value = u.role || 'subscriber';
      openModal('modal-edit-user');
    }

    async function handleUpdateUser(e) {
      e.preventDefault();
      const id = document.getElementById('edit-user-id').value;
      const payload = {
        full_name: document.getElementById('edit-user-name').value.trim(),
        store_name: document.getElementById('edit-user-store').value.trim(),
        phone: document.getElementById('edit-user-phone').value.trim(),
        role: document.getElementById('edit-user-role').value
      };

      try {
        const res = await apiFetch('/api/admin/users/' + id, {
          method: 'PUT',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تعديل البيانات');

        closeModal('modal-edit-user');
        showToast('تم تحديث بيانات المشترك بنجاح');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function deleteUserPrompt(id, name) {
      if (!confirm('هل أنت متأكد من حذف المشترك (' + name + ')؟ سيتم إلغاء وصوله نهائياً.')) return;
      try {
        const res = await apiFetch('/api/admin/users/' + id, { method: 'DELETE' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حذف المشترك');
        showToast('تم حذف المشترك بنجاح');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // CORE TAB 3: ACTIVATION CODES
    // =========================================================================
    async function loadActivationCodes() {
      const tbody = document.getElementById('codes-table-body');
      try {
        const res = await apiFetch('/api/admin/activation-codes');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-slate-400 font-bold">لا توجد أكواد تفعيل مصدرة</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(c => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-black text-brand text-xs">${escapeHtml(c.code)}</td>
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(c.plan_name || 'باقة')}</td>
              <td class="py-3 px-3 font-bold text-slate-600">${c.days} يوم</td>
              <td class="py-3 px-3">${c.is_used ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600">مستخدم</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">متاح</span>'}</td>
              <td class="py-3 px-3 text-slate-500 font-mono text-[11px]">${escapeHtml(c.used_by_email || '—')}</td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatDate(c.created_at)}</td>
              <td class="py-3 px-3 text-center">
                <button onclick="copyToClipboard('${c.code}')" class="p-1.5 text-slate-500 hover:text-brand"><i class="fa-solid fa-copy"></i></button>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل الأكواد</td></tr>';
      }
    }

    function copyToClipboard(text) {
      navigator.clipboard.writeText(text);
      showToast('تم نسخ الكود للحافظة');
    }

    // =========================================================================
    // CORE TAB 4: WALLET & ZAINCASH
    // =========================================================================
    async function loadZainCashTransactions() {
      const tbody = document.getElementById('zaincash-table-body');
      try {
        const res = await apiFetch('/api/admin/zaincash-transactions');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-slate-400 font-bold">لا توجد حركات زين كاش مسجلة</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(tx => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold text-slate-600">#${tx.id}</td>
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(tx.user_email || 'مشترك')}</td>
              <td class="py-3 px-3 font-black text-emerald-600">${formatIQD(tx.amount)}</td>
              <td class="py-3 px-3"><span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">${escapeHtml(tx.status)}</span></td>
              <td class="py-3 px-3 font-mono text-[11px] text-slate-500">${escapeHtml(tx.zaincash_id || '—')}</td>
              <td class="py-3 px-3 text-[11px] text-slate-400">${formatBaghdad(tx.created_at)}</td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل حركات زين كاش</td></tr>';
      }
    }

    function openWalletAdjustModal(userId) {
      const u = allUsers.find(x => x.id === userId);
      if (!u) return;
      document.getElementById('wallet-adjust-user-id').value = u.id;
      document.getElementById('wallet-adjust-user-name').innerText = (u.store_name || u.full_name) + ' (' + u.email + ')';
      document.getElementById('wallet-adjust-current-balance').innerText = 'الرصيد الحالي: ' + formatIQD(u.wallet_balance || 0);
      document.getElementById('wallet-adjust-amount').value = '';
      document.getElementById('wallet-adjust-notes').value = '';
      openModal('modal-wallet-adjust');
    }

    async function handleWalletAdjustment(e) {
      e.preventDefault();
      const userId = document.getElementById('wallet-adjust-user-id').value;
      const type = document.getElementById('wallet-adjust-type').value;
      const amount = parseInt(document.getElementById('wallet-adjust-amount').value, 10);
      const notes = document.getElementById('wallet-adjust-notes').value.trim();

      try {
        const res = await apiFetch('/api/admin/users/' + userId + '/wallet', {
          method: 'POST',
          body: JSON.stringify({ type, amount, notes })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تعديل الرصيد');

        closeModal('modal-wallet-adjust');
        showToast('تم تنفيذ القيد المالي بنجاح');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // CORE TAB 5: CARDS & VAULT
    // =========================================================================
    async function loadDigitalCards() {
      const grid = document.getElementById('cards-grid');
      try {
        const res = await apiFetch('/api/admin/cards');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            grid.innerHTML = '<div class="col-span-full py-12 text-center text-slate-400 font-bold">لا توجد بطاقات مضافة في الخزنة</div>';
            return;
          }
          grid.innerHTML = data.map(c => `
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
              <div class="flex items-center justify-between">
                <div class="font-black text-sm text-slate-800">${escapeHtml(c.name)}</div>
                <span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand">${escapeHtml(c.category_id || 'عام')}</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-2xl flex items-center justify-between text-xs">
                <div><span class="text-slate-400 block text-[10px]">سعر البيع</span><span class="font-black text-emerald-600">${formatIQD(c.price)}</span></div>
                <div><span class="text-slate-400 block text-[10px]">التكلفة</span><span class="font-black text-slate-600">${formatIQD(c.cost)}</span></div>
                <div><span class="text-slate-400 block text-[10px]">المتاح</span><span class="font-black text-brand">${c.available_count || 0}</span></div>
              </div>
            </div>
          `).join('');
        }
      } catch (err) {
        grid.innerHTML = '<div class="col-span-full py-12 text-center text-rose-500 font-bold">خطأ في تحميل البطاقات</div>';
      }
    }

    async function handleSaveDigitalCard(e) {
      e.preventDefault();
      const payload = {
        name: document.getElementById('card-name').value.trim(),
        cost: parseInt(document.getElementById('card-cost').value, 10),
        price: parseInt(document.getElementById('card-price').value, 10),
        category_id: document.getElementById('card-category').value.trim()
      };
      try {
        const res = await apiFetch('/api/admin/cards', { method: 'POST', body: JSON.stringify(payload) });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ البطاقة');
        closeModal('modal-add-card');
        showToast('تمت إضافة فئة البطاقة بنجاح');
        loadDigitalCards();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // CORE TAB 6: AUDIT TRAIL
    // =========================================================================
    async function loadAuditLogs() {
      const tbody = document.getElementById('audit-table-body');
      try {
        const res = await apiFetch('/api/admin/audit-logs');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-slate-400 font-bold">سجل العمليات فارغ حالياً</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(log => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold text-slate-500">#${log.id}</td>
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(log.actor_email || 'النظام')}</td>
              <td class="py-3 px-3 font-mono font-black text-brand">${escapeHtml(log.action)}</td>
              <td class="py-3 px-3 text-slate-600">${escapeHtml(log.target_type)}:${log.target_id}</td>
              <td class="py-3 px-3 text-slate-500 text-[11px] max-w-xs truncate">${escapeHtml(JSON.stringify(log.details || {}))}</td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatBaghdad(log.created_at)}</td>
              <td class="py-3 px-3 text-center">
                <button onclick="rollbackAudit(${log.id})" title="عكس العملية" class="px-2 py-1 bg-rose-50 hover:bg-rose-100 text-rose-600 rounded-lg text-xs font-bold transition flex items-center justify-center gap-1 mx-auto">
                  <i class="fa-solid fa-rotate-left"></i><span>عكس</span>
                </button>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل سجل العمليات</td></tr>';
      }
    }
