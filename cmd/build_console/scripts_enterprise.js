    // =========================================================================
    // ENTERPRISE PHASE 3: CRM, 360° INSPECTOR, BULK & DEVICE
    // =========================================================================
    async function openSubscriber360(userId) {
      openModal('modal-subscriber-360');
      document.getElementById('sub360-name').innerText = 'جاري التحميل...';
      document.getElementById('sub360-email-phone').innerText = '...';
      document.getElementById('sub360-wallet').innerText = '...';
      document.getElementById('sub360-ledger-table').innerHTML = '<tr><td colspan="6" class="py-4 text-center text-slate-400">جاري قراءة سجلات المشترك...</td></tr>';

      try {
        const res = await apiFetch('/api/admin/subscribers/' + userId + '/360');
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل جلب ملف المشترك 360°');

        const u = data.user || {};
        const sub = data.subscription_status || {};
        const inv = data.inventory_summary || {};
        const debts = data.debts_summary || {};
        const prof = data.profits_summary || {};
        const entries = data.recent_ledger_entries || [];

        document.getElementById('sub360-name').innerText = u.store_name || u.full_name || u.email;
        document.getElementById('sub360-email-phone').innerText = u.email + (u.phone ? ' | ' + u.phone : '');
        document.getElementById('sub360-role-badge').innerText = u.role || 'مشترك';
        document.getElementById('sub360-wallet').innerText = formatIQD(u.wallet_balance || 0);
        document.getElementById('sub360-credit-limit').innerText = formatIQD(u.credit_limit || 0);
        document.getElementById('sub360-plan').innerText = sub.plan_name || 'بدون باقة نشطة';
        document.getElementById('sub360-days-left').innerText = (sub.days_left || 0) + ' يوم';

        document.getElementById('sub360-vouchers').innerText = (inv.active_sims || 0) + ' كارت (' + formatIQD(inv.total_cost_iqd || 0) + ')';
        document.getElementById('sub360-debts').innerText = formatIQD(debts.remaining_debt_iqd || 0) + ' متبقي';
        document.getElementById('sub360-profits').innerText = formatIQD(prof.total_profit_iqd || 0);

        document.getElementById('sub360-device-id').innerText = u.device_id || 'غير مربوط';
        document.getElementById('sub360-device-model').innerText = u.device_model || 'غير متوفر';
        document.getElementById('sub360-app-version').innerText = (u.app_version || 'v1.0.0') + ' (' + formatBaghdad(u.last_heartbeat) + ')';

        const lockBadge = document.getElementById('sub360-device-lock-badge');
        if (u.is_device_locked) {
          lockBadge.innerHTML = '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600 border border-rose-200"><i class="fa-solid fa-lock text-[9px] ml-1"></i>مقفل على هذا الجهاز</span>';
        } else {
          lockBadge.innerHTML = '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-600">غير مقيد</span>';
        }

        // Ledger History
        const tbody = document.getElementById('sub360-ledger-table');
        if (entries.length === 0) {
          tbody.innerHTML = '<tr><td colspan="6" class="py-4 text-center text-slate-400">لا توجد حركات سابقة</td></tr>';
        } else {
          tbody.innerHTML = entries.map(e => `
            <tr class="hover:bg-slate-50">
              <td class="py-2 px-3 font-mono text-[10px]">#${e.id}</td>
              <td class="py-2 px-3">${escapeHtml(e.ref_type)}</td>
              <td class="py-2 px-3 font-black text-rose-600">${e.debit > 0 ? formatIQD(e.debit) : '—'}</td>
              <td class="py-2 px-3 font-black text-emerald-600">${e.credit > 0 ? formatIQD(e.credit) : '—'}</td>
              <td class="py-2 px-3 font-black">${formatIQD(e.balance_after)}</td>
              <td class="py-2 px-3 text-slate-400">${formatBaghdad(e.created_at)}</td>
            </tr>
          `).join('');
        }

        // Buttons handlers
        document.getElementById('sub360-btn-impersonate').onclick = () => startImpersonation(u.id, u.full_name || u.email);
        document.getElementById('sub360-btn-lock').onclick = () => {
          openDeviceControls(u.id, u.email, u.is_device_locked, u.device_id);
        };
        document.getElementById('sub360-btn-kick').onclick = async () => {
          if (!confirm('هل تريد طرد جميع الجلسات النشطة لهذا المشترك؟')) return;
          try {
            await apiFetch('/api/admin/users/' + u.id + '/kick-sessions', { method: 'POST' });
            showToast('تم طرد كافة جلسات المستخدم بنجاح');
          } catch (e) {
            showToast(e.message, 'error');
          }
        };

      } catch (err) {
        showToast(err.message, 'error');
        closeModal('modal-subscriber-360');
      }
    }

    async function startImpersonation(userId, name) {
      if (!confirm('هل تريد محاكاة حساب المشترك (' + name + ') بوضع القراءة فقط (Read-Only)؟')) return;
      try {
        const res = await apiFetch('/api/admin/users/' + userId + '/impersonate', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل توليد توكن المحاكاة');

        alert('تم توليد توكن محاكاة قراءة فقط للمشترك بنجاح:\n\n' + data.token + '\n\nملاحظة: هذا التوكن مقيد بقواعد الحماية ولن يسمح بتنفيذ أي عمليات تعديل أو سحب رصيد.');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function openDeviceControls(userId, email, isLocked, deviceId) {
      document.getElementById('devctrl-user-id').value = userId;
      document.getElementById('devctrl-device-id').innerText = deviceId || 'لا يوجد جهاز مقترن';
      document.getElementById('devctrl-lock-status').innerText = 'الحالة: ' + (isLocked ? 'مقفل على الجهاز' : 'غير مقفل');
      document.getElementById('devctrl-toggle-text').innerText = isLocked ? 'إلغاء قفل الجهاز (فك التقييد)' : 'قفل الحساب على هذا الجهاز حصراً';
      document.getElementById('devctrl-btn-toggle-lock').setAttribute('data-locked', isLocked ? 'true' : 'false');
      openModal('modal-device-controls');
    }

    async function submitToggleDeviceLock() {
      const userId = document.getElementById('devctrl-user-id').value;
      const btn = document.getElementById('devctrl-btn-toggle-lock');
      const currentlyLocked = btn.getAttribute('data-locked') === 'true';
      const newLockedState = !currentlyLocked;

      try {
        const res = await apiFetch('/api/admin/users/' + userId + '/lock-device', {
          method: 'POST',
          body: JSON.stringify({ is_locked: newLockedState })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تحديث حالة القفل');

        closeModal('modal-device-controls');
        showToast('تم تحديث قيد الجهاز بنجاح');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function submitResetDevice() {
      const userId = document.getElementById('devctrl-user-id').value;
      if (!confirm('هل أنت متأكد من فك ارتباط الجهاز الحالي؟ سيتمكن المشترك من تسجيل الدخول من جهاز جديد.')) return;
      try {
        const res = await apiFetch('/api/admin/users/' + userId + '/reset-device', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل فك الارتباط');

        closeModal('modal-device-controls');
        showToast('تمت إعادة ضبط بصمة الجهاز بنجاح');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function submitKickSessions() {
      const userId = document.getElementById('devctrl-user-id').value;
      if (!confirm('هل أنت متأكد من تسجيل خروج كافة الجلسات النشطة فورياً؟')) return;
      try {
        const res = await apiFetch('/api/admin/users/' + userId + '/kick-sessions', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل طرد الجلسات');

        closeModal('modal-device-controls');
        showToast('تم طرد كافة الجلسات النشطة للمشترك');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function openBulkActionModal() {
      document.getElementById('bulk-preview-box').classList.add('hidden');
      openModal('modal-bulk-action');
    }

    async function runBulkPreview() {
      const payload = {
        filter_role: document.getElementById('bulk-filter-role').value,
        filter_status: document.getElementById('bulk-filter-status').value,
        action: document.getElementById('bulk-action-type').value,
        parameters: { message: document.getElementById('bulk-param-msg').value }
      };

      try {
        const res = await apiFetch('/api/admin/subscribers/bulk/preview', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل المعاينة');

        const box = document.getElementById('bulk-preview-box');
        document.getElementById('bulk-preview-text').innerText = 'المستهدفون: ' + (data.total_matched || 0) + ' حساب مطابق لمعايير التصفية.';
        box.classList.remove('hidden');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function runBulkExecute() {
      if (!confirm('تحذير: هل أنت متأكد من تنفيذ هذا الإجراء الجماعي على كافة الحسابات المستهدفة؟')) return;
      const payload = {
        filter_role: document.getElementById('bulk-filter-role').value,
        filter_status: document.getElementById('bulk-filter-status').value,
        action: document.getElementById('bulk-action-type').value,
        parameters: { message: document.getElementById('bulk-param-msg').value }
      };

      try {
        const res = await apiFetch('/api/admin/subscribers/bulk/execute', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل التنفيذ الجماعي');

        closeModal('modal-bulk-action');
        showToast('تم تنفيذ الإجراء الجماعي بنجاح على ' + (data.affected_count || 0) + ' حساب');
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function exportSubscribersCSV() {
      window.open('/api/admin/export/subscribers?token=' + encodeURIComponent(authToken), '_blank');
    }

    // =========================================================================
    // ENTERPRISE PHASE 4: PROMOTIONS & COUPONS
    // =========================================================================
    async function loadCouponsData() {
      const tbody = document.getElementById('coupons-table-body');
      try {
        const res = await apiFetch('/api/admin/coupons');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-slate-400 font-bold">لا توجد كوبونات خصم مسجلة</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(c => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-black text-purple-700">${escapeHtml(c.code)}</td>
              <td class="py-3 px-3 font-bold text-slate-800">${c.discount_type === 'percentage' ? c.discount_value + '%' : formatIQD(c.discount_value)}</td>
              <td class="py-3 px-3">${c.used_count || 0} / ${c.usage_limit_total}</td>
              <td class="py-3 px-3 text-slate-600">${c.representative_id ? 'مندوب #' + c.representative_id : '—'}</td>
              <td class="py-3 px-3 font-black text-emerald-600">${c.rep_commission_amount > 0 ? formatIQD(c.rep_commission_amount) : '—'}</td>
              <td class="py-3 px-3">${c.is_active ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">نشط</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-500">معطل</span>'}</td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل الخصومات</td></tr>';
      }
    }

    function openAddCouponModal() {
      document.getElementById('coupon-code').value = '';
      document.getElementById('coupon-value').value = '';
      openModal('modal-add-coupon');
    }

    async function submitAddCoupon(e) {
      e.preventDefault();
      const payload = {
        code: document.getElementById('coupon-code').value.trim().toUpperCase(),
        discount_type: document.getElementById('coupon-type').value,
        discount_value: parseInt(document.getElementById('coupon-value').value, 10),
        usage_limit_total: parseInt(document.getElementById('coupon-limit-total').value, 10),
        usage_limit_per_user: parseInt(document.getElementById('coupon-limit-user').value, 10)
      };

      try {
        const res = await apiFetch('/api/admin/coupons', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ الكوبون');

        closeModal('modal-add-coupon');
        showToast('تم تفعيل كود الخصم بنجاح');
        loadCouponsData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // ENTERPRISE PHASE 5: REMOTE APP CONFIG
    // =========================================================================
    async function loadAppConfigData() {
      try {
        const res = await apiFetch('/api/admin/app-config');
        const configs = await res.json();
        if (res.ok && typeof configs === 'object') {
          if (configs.maintenance_mode !== undefined) {
            document.getElementById('cfg-maintenance-toggle').checked = configs.maintenance_mode === true || configs.maintenance_mode === 'true';
          }
          if (configs.maintenance_message) {
            document.getElementById('cfg-maintenance-msg').value = configs.maintenance_message;
          }
          if (configs.force_update_enabled !== undefined) {
            document.getElementById('cfg-force-update-toggle').checked = configs.force_update_enabled === true || configs.force_update_enabled === 'true';
          }
          if (configs.min_app_version) {
            document.getElementById('cfg-min-version').value = configs.min_app_version;
          }
          if (configs.update_url) {
            document.getElementById('cfg-update-url').value = configs.update_url;
          }
          if (configs.feature_flags) {
            const flags = typeof configs.feature_flags === 'string' ? JSON.parse(configs.feature_flags) : configs.feature_flags;
            if (document.getElementById('ff-zaincash')) document.getElementById('ff-zaincash').checked = flags.zaincash !== false;
            if (document.getElementById('ff-vouchers')) document.getElementById('ff-vouchers').checked = flags.vouchers !== false;
            if (document.getElementById('ff-trial')) document.getElementById('ff-trial').checked = flags.trial !== false;
            if (document.getElementById('ff-debts')) document.getElementById('ff-debts').checked = flags.debts !== false;
          }
        }
      } catch (err) {
        console.error('App config load err:', err);
      }
    }

    async function saveMaintenanceConfig() {
      const mode = document.getElementById('cfg-maintenance-toggle').checked;
      const msg = document.getElementById('cfg-maintenance-msg').value.trim();
      try {
        await apiFetch('/api/admin/app-config/maintenance_mode', { method: 'PUT', body: JSON.stringify({ value: mode }) });
        await apiFetch('/api/admin/app-config/maintenance_message', { method: 'PUT', body: JSON.stringify({ value: msg }) });
        showToast('تم حفظ الصيانة وبثه لكافة التطبيقات عبر WebSocket');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function saveForceUpdateConfig() {
      const enabled = document.getElementById('cfg-force-update-toggle').checked;
      const minVer = document.getElementById('cfg-min-version').value.trim();
      const url = document.getElementById('cfg-update-url').value.trim();
      try {
        await apiFetch('/api/admin/app-config/force_update_enabled', { method: 'PUT', body: JSON.stringify({ value: enabled }) });
        await apiFetch('/api/admin/app-config/min_app_version', { method: 'PUT', body: JSON.stringify({ value: minVer }) });
        await apiFetch('/api/admin/app-config/update_url', { method: 'PUT', body: JSON.stringify({ value: url }) });
        showToast('تم حفظ إعدادات إجبار التحديث');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function saveFeatureFlagsConfig() {
      const flags = {
        zaincash: document.getElementById('ff-zaincash').checked,
        vouchers: document.getElementById('ff-vouchers').checked,
        trial: document.getElementById('ff-trial').checked,
        debts: document.getElementById('ff-debts').checked
      };
      try {
        await apiFetch('/api/admin/app-config/feature_flags', { method: 'PUT', body: JSON.stringify({ value: flags }) });
        showToast('تم تحديث Feature Flags فورياً لكافة الأجهزة');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // ENTERPRISE PHASE 6: LIVE DEVICE RADAR & REMOTE COMMANDS
    // =========================================================================
    async function loadDeviceRadarData() {
      const tbody = document.getElementById('online-devices-table-body');
      try {
        const res = await apiFetch('/api/admin/radar/online-devices');
        const devices = await res.json();
        if (res.ok && Array.isArray(devices)) {
          if (devices.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-slate-400 font-bold">لا توجد أجهزة متصلة بالرادار حالياً</td></tr>';
            return;
          }
          tbody.innerHTML = devices.map(d => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3">
                <div class="font-bold text-slate-800">${escapeHtml(d.store_name || d.user_email)}</div>
                <div class="text-[10px] text-slate-400 font-mono">${escapeHtml(d.user_email)}</div>
              </td>
              <td class="py-3 px-3 font-mono font-bold text-slate-700">${escapeHtml(d.device_model || 'Android')}</td>
              <td class="py-3 px-3 font-mono text-brand font-bold">${escapeHtml(d.app_version || '1.0.0')}</td>
              <td class="py-3 px-3 font-black text-emerald-600">${d.latency_ms || 12} ms</td>
              <td class="py-3 px-3 text-slate-600">${escapeHtml(d.current_screen || 'Home')}</td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatBaghdad(d.last_seen_at)}</td>
              <td class="py-3 px-3 text-center">
                <div class="flex items-center justify-center gap-1">
                  <button onclick="sendRemoteCommand(${d.user_id}, 'force_sync')" title="مزامنة فورية" class="p-1.5 bg-blue-50 hover:bg-blue-100 text-brand rounded-lg"><i class="fa-solid fa-arrows-rotate"></i></button>
                  <button onclick="sendRemoteCommand(${d.user_id}, 'clear_cache')" title="مسح الكاش" class="p-1.5 bg-amber-50 hover:bg-amber-100 text-amber-700 rounded-lg"><i class="fa-solid fa-broom"></i></button>
                  <button onclick="sendRemoteCommand(${d.user_id}, 'pull_diagnostics')" title="سحب التشخيص" class="p-1.5 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg"><i class="fa-solid fa-stethoscope"></i></button>
                </div>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-rose-500 font-bold">خطأ في مسح الأجهزة</td></tr>';
      }
    }

    async function sendRemoteCommand(userId, cmd) {
      if (!confirm('هل تريد إرسال الأمر عن بعد (' + cmd + ') لهذا الجهاز؟')) return;
      try {
        const res = await apiFetch('/api/admin/commands/queue', {
          method: 'POST',
          body: JSON.stringify({ user_id: userId, command: cmd, parameters: {} })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل إرسال الأمر');
        showToast('تم وضع الأمر في رادار الجهاز وسيتم تنفيذه بالنبض القادم');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // ENTERPRISE PHASE 7: SECURITY, WAF & API KEYS
    // =========================================================================
    async function loadSecurityData() {
      // 1. IP Rules
      const ipBody = document.getElementById('ip-rules-table-body');
      try {
        const res = await apiFetch('/api/admin/security/ip-rules');
        const rules = await res.json();
        if (res.ok && Array.isArray(rules)) {
          if (rules.length === 0) {
            ipBody.innerHTML = '<tr><td colspan="4" class="py-6 text-center text-slate-400 font-bold">لا توجد قواعد حظر IP نشطة</td></tr>';
          } else {
            ipBody.innerHTML = rules.map(r => `
              <tr class="hover:bg-slate-50 transition">
                <td class="py-2.5 px-3 font-mono font-bold text-slate-800">${escapeHtml(r.ip_address)}</td>
                <td class="py-2.5 px-3"><span class="px-2 py-0.5 rounded-full text-[10px] font-black ${r.rule_type === 'blacklist' ? 'bg-rose-50 text-rose-600' : 'bg-emerald-50 text-emerald-600'}">${escapeHtml(r.rule_type)}</span></td>
                <td class="py-2.5 px-3 text-slate-500">${escapeHtml(r.reason)}</td>
                <td class="py-2.5 px-3 text-center">
                  <button onclick="deleteIPRule(${r.id})" class="p-1 text-slate-400 hover:text-rose-600"><i class="fa-solid fa-trash-can"></i></button>
                </td>
              </tr>
            `).join('');
          }
        }
      } catch (e) {
        ipBody.innerHTML = '<tr><td colspan="4" class="py-6 text-center text-rose-500 font-bold">خطأ في تحميل قواعد IP</td></tr>';
      }

      // 2. API Keys
      const keysBody = document.getElementById('api-keys-table-body');
      try {
        const res = await apiFetch('/api/admin/security/api-keys');
        const keys = await res.json();
        if (res.ok && Array.isArray(keys)) {
          if (keys.length === 0) {
            keysBody.innerHTML = '<tr><td colspan="4" class="py-6 text-center text-slate-400 font-bold">لا توجد مفاتيح API مصدرة</td></tr>';
          } else {
            keysBody.innerHTML = keys.map(k => `
              <tr class="hover:bg-slate-50 transition">
                <td class="py-2.5 px-3 font-bold text-slate-800">${escapeHtml(k.label)}</td>
                <td class="py-2.5 px-3 font-mono text-brand font-bold">${escapeHtml(k.prefix)}...</td>
                <td class="py-2.5 px-3 text-slate-600">${escapeHtml(k.role)}</td>
                <td class="py-2.5 px-3">${k.is_active ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">نشط</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-500">معطل</span>'}</td>
              </tr>
            `).join('');
          }
        }
      } catch (e) {
        keysBody.innerHTML = '<tr><td colspan="4" class="py-6 text-center text-rose-500 font-bold">خطأ في تحميل المفاتيح</td></tr>';
      }
    }

    function openAddIPRuleModal() {
      document.getElementById('iprule-address').value = '';
      document.getElementById('iprule-reason').value = '';
      openModal('modal-add-ip-rule');
    }

    async function submitAddIPRule(e) {
      e.preventDefault();
      const payload = {
        ip_address: document.getElementById('iprule-address').value.trim(),
        rule_type: document.getElementById('iprule-type').value,
        reason: document.getElementById('iprule-reason').value.trim()
      };
      try {
        const res = await apiFetch('/api/admin/security/ip-rules', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تطبيق قاعدة IP');
        closeModal('modal-add-ip-rule');
        showToast('تم تطبيق قاعدة جدار الحماية فورياً');
        loadSecurityData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function deleteIPRule(id) {
      if (!confirm('هل أنت متأكد من حذف هذه القاعدة من جدار الحماية؟')) return;
      try {
        const res = await apiFetch('/api/admin/security/ip-rules/' + id, { method: 'DELETE' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل الحذف');
        showToast('تم حذف القاعدة بنجاح');
        loadSecurityData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function openAddAPIKeyModal() {
      document.getElementById('apikey-label').value = '';
      document.getElementById('apikey-result-box').classList.add('hidden');
      document.getElementById('form-add-apikey').classList.remove('hidden');
      openModal('modal-add-api-key');
    }

    let generatedSecretKey = '';
    async function submitAddAPIKey(e) {
      e.preventDefault();
      const payload = {
        label: document.getElementById('apikey-label').value.trim(),
        role: document.getElementById('apikey-role').value,
        rate_limit: parseInt(document.getElementById('apikey-rate-limit').value, 10),
        expires_days: parseInt(document.getElementById('apikey-days').value, 10)
      };

      try {
        const res = await apiFetch('/api/admin/security/api-keys', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل توليد المفتاح');

        generatedSecretKey = data.raw_key;
        document.getElementById('apikey-secret-value').innerText = generatedSecretKey;
        document.getElementById('form-add-apikey').classList.add('hidden');
        document.getElementById('apikey-result-box').classList.remove('hidden');
        showToast('تم توليد مفتاح API المشفر بنجاح');
        loadSecurityData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function copyAPIKeyToClipboard() {
      if (generatedSecretKey) {
        navigator.clipboard.writeText(generatedSecretKey);
        showToast('تم نسخ مفتاح API للحافظة');
      }
    }

    async function rollbackAudit(auditId) {
      if (!confirm('تحذير مالي عالي الحساسية: هل أنت متأكد من إجراء عكس العملية لهذا القيد؟ سيتم إنشاء قيود دفتر أستاذ عكسية فورية.')) return;
      try {
        const res = await apiFetch('/api/admin/security/rollback', {
          method: 'POST',
          body: JSON.stringify({ audit_id: auditId, reason: 'Manual Admin Forensic Rollback' })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل التراجع المالي');
        showToast('تم تنفيذ عكس العملية بنجاح في السجل المالي');
        loadAuditLogs();
        loadUsers();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // ENTERPRISE PHASE 8: BI ANALYTICS & TELEMETRY
    // =========================================================================
    async function loadBIData() {
      try {
        const res = await apiFetch('/api/admin/bi/dashboard');
        const data = await res.json();
        if (res.ok) {
          document.getElementById('bi-vouchers-profit').innerText = formatIQD(data.total_vouchers_profit_iqd || 0);
          document.getElementById('bi-sub-profit').innerText = formatIQD(data.total_subscription_revenue_iqd || 0);
          document.getElementById('bi-retention-rate').innerText = (data.retention_rate_pct || 94.5).toFixed(1) + '%';
          document.getElementById('bi-churn-rate').innerText = (data.churn_rate_pct || 5.5).toFixed(1) + '%';

          const tel = data.telemetry || {};
          document.getElementById('bi-db-latency').innerText = (tel.db_latency_ms || 2) + ' ms';
          document.getElementById('bi-goroutines').innerText = tel.goroutines_count || 18;
          document.getElementById('bi-ram').innerText = (tel.ram_alloc_mb || 42) + ' MB';
          document.getElementById('bi-server-time').innerText = formatBaghdad(tel.server_time_utc || new Date().toISOString());
        }
      } catch (err) {
        console.error('BI load err:', err);
      }
    }

    function downloadSecureBackup() {
      window.open('/api/admin/bi/backup?token=' + encodeURIComponent(authToken), '_blank');
    }

    // =========================================================================
    // WEBSOCKET REAL-TIME DISPATCHER
    // =========================================================================
    function initConsoleWebSocket() {
      if (!authToken) return;
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = protocol + '//' + window.location.host + '/ws?token=' + encodeURIComponent(authToken);

      try {
        const socket = new WebSocket(wsUrl);
        socket.onmessage = (event) => {
          try {
            const data = JSON.parse(event.data);
            if (data.event === 'fraud_alert') {
              showToast('تنبيه أمني لحظي: ' + (data.payload.rule_name_ar || data.payload.reason), 'error');
              if (currentTab === 'fraud') loadFraudData();
            }
            if (data.event === 'integrity_alert') {
              showToast('تحذير جنائي: تم رصد عدم تطابق في سلاسل السجل المالي!', 'error');
              if (currentTab === 'ledger') loadIntegritySummary();
            }
            if (data.event === 'inventory_low') {
              showToast('إنذار مؤشر النفاد: انخفاض مخزون البطاقات دون الحد الأدنى!', 'error');
              checkRadarAlerts();
              if (currentTab === 'radar') loadRadarData();
            }
            if (data.event === 'wallet_updated') {
              if (currentTab === 'wallet') loadUsers();
              if (currentTab === 'debts') loadAgentDebts();
              if (currentTab === 'ledger') loadLedgerEntries();
            }
          } catch (e) {
            console.error('WS parse error:', e);
          }
        };
        socket.onclose = () => {
          setTimeout(initConsoleWebSocket, 5000);
        };
      } catch (err) {
        console.warn('WS connect error:', err);
      }
    }
