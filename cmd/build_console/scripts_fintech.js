    // =========================================================================
    // FINTECH TAB 7: DOUBLE-ENTRY LEDGER & INTEGRITY
    // =========================================================================
    async function loadLedgerEntries() {
      const tbody = document.getElementById('ledger-table-body');
      try {
        const res = await apiFetch('/api/admin/fintech/ledger');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="8" class="py-8 text-center text-slate-400 font-bold">لا توجد حركات محاسبية في السجل المالي</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(entry => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold text-slate-600">#${entry.id} <span class="text-[10px] text-slate-400 block">${entry.tx_id}</span></td>
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(entry.account_name || 'حساب #' + entry.account_id)}</td>
              <td class="py-3 px-3 font-black text-rose-600">${entry.debit > 0 ? formatIQD(entry.debit) : '—'}</td>
              <td class="py-3 px-3 font-black text-emerald-600">${entry.credit > 0 ? formatIQD(entry.credit) : '—'}</td>
              <td class="py-3 px-3 font-black text-slate-800">${formatIQD(entry.balance_after)}</td>
              <td class="py-3 px-3"><span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand">${escapeHtml(entry.ref_type)}</span></td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatBaghdad(entry.created_at)}</td>
              <td class="py-3 px-3 font-mono text-[10px] text-slate-400 truncate max-w-xs" title="${entry.hash}">${entry.hash ? entry.hash.substring(0, 16) + '...' : '—'}</td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="8" class="py-8 text-center text-rose-500 font-bold">خطأ في قراءة السجل المالي</td></tr>';
      }
    }

    async function loadIntegritySummary() {
      try {
        const res = await apiFetch('/api/admin/fintech/integrity/alerts');
        const alerts = await res.json();
        const card = document.getElementById('integrity-status-card');
        const title = document.getElementById('integrity-status-title');
        const desc = document.getElementById('integrity-status-desc');
        document.getElementById('integrity-last-checked').innerText = new Date().toLocaleTimeString('ar-IQ', { timeZone: 'Asia/Baghdad' });

        if (res.ok && Array.isArray(alerts) && alerts.length > 0) {
          card.className = 'p-4 rounded-3xl bg-rose-50 border border-rose-200 flex flex-col md:flex-row md:items-center justify-between gap-4';
          title.className = 'font-black text-sm text-rose-900';
          title.innerText = 'تنبيه جنائي: تم رصد عدم تطابق في سلاسل السجل المالي!';
          desc.className = 'text-xs text-rose-700 mt-0.5';
          desc.innerText = 'يوجد ' + alerts.length + ' تنبيهات جنائية بخصوص سلامة الأرصدة أو سلاسل الهاش. يرجى مراجعة سجل الحركات فوراً.';
        } else {
          card.className = 'p-4 rounded-3xl bg-emerald-50/80 border border-emerald-200 flex flex-col md:flex-row md:items-center justify-between gap-4';
          title.className = 'font-black text-sm text-emerald-900';
          title.innerText = 'سلاسل السجل المالي سليمة 100% ومطابقة محاسبياً';
          desc.className = 'text-xs text-emerald-700 mt-0.5';
          desc.innerText = 'تم التحقق من كافة القيود، وتطابق الأرصدة التراكمية، ولا توجد أي انقطاعات أو تلاعب بقواعد البيانات.';
        }
      } catch (e) {
        console.error('Integrity summary err:', e);
      }
    }

    async function triggerIntegrityCheck() {
      const btn = document.getElementById('btn-verify-integrity');
      btn.disabled = true;
      btn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i><span>جاري الفحص الجنائي...</span>';
      try {
        const res = await apiFetch('/api/admin/fintech/integrity/verify', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تشغيل الفحص الجنائي');
        showToast(data.message || 'تم اكتمال الفحص الجنائي لسلاسل الهاش بنجاح');
        loadIntegritySummary();
      } catch (err) {
        showToast(err.message, 'error');
      } finally {
        btn.disabled = false;
        btn.innerHTML = '<i class="fa-solid fa-fingerprint"></i><span>بدء الفحص الجنائي لسلاسل الهاش</span>';
      }
    }

    // =========================================================================
    // FINTECH TAB 8: FRAUD ENGINE
    // =========================================================================
    async function loadFraudData() {
      // 1. Load Rules
      try {
        const res = await apiFetch('/api/admin/fintech/fraud/rules');
        const rules = await res.json();
        const container = document.getElementById('fraud-rules-list');
        if (res.ok && Array.isArray(rules)) {
          container.innerHTML = rules.map(r => `
            <div class="p-3 bg-slate-50 rounded-2xl border border-slate-100 flex items-center justify-between">
              <div>
                <span class="font-bold text-xs text-slate-800 block">${escapeHtml(r.rule_name_ar)}</span>
                <span class="text-[10px] text-slate-400 block">${r.threshold_count} أحداث خلال ${r.time_window_seconds} ثانية</span>
              </div>
              <input type="checkbox" ${r.is_active ? 'checked' : ''} onchange="toggleFraudRule(${r.id}, this.checked)" class="w-5 h-5 accent-brand cursor-pointer">
            </div>
          `).join('');
        }
      } catch (e) {
        console.error('Fraud rules err:', e);
      }

      // 2. Load Frozen Wallets
      try {
        const res = await apiFetch('/api/admin/fintech/fraud/events');
        const data = await res.json();
        const tbody = document.getElementById('frozen-wallets-table-body');
        const badge = document.getElementById('frozen-count-badge');
        const navBadge = document.getElementById('badge-fraud-alerts');

        if (res.ok && Array.isArray(data)) {
          const frozen = data.filter(ev => ev.action_taken === 'freeze_wallet' || ev.is_frozen);
          badge.innerText = frozen.length + ' حساب';
          if (frozen.length > 0) {
            navBadge.innerText = frozen.length;
            navBadge.classList.remove('hidden');
            tbody.innerHTML = frozen.map(f => `
              <tr class="hover:bg-slate-50 transition">
                <td class="py-2.5 px-3 font-bold text-slate-800">${escapeHtml(f.user_email || 'حساب #' + f.user_id)}</td>
                <td class="py-2.5 px-3 text-rose-600 font-bold">${escapeHtml(f.reason || 'اشتباه احتيالي')}</td>
                <td class="py-2.5 px-3 text-center">
                  <button onclick="unfreezeWallet(${f.user_id})" class="px-2.5 py-1 bg-emerald-50 text-emerald-700 hover:bg-emerald-100 rounded-lg text-xs font-bold transition">فك التجميد</button>
                </td>
              </tr>
            `).join('');
          } else {
            navBadge.classList.add('hidden');
            tbody.innerHTML = '<tr><td colspan="3" class="py-6 text-center text-slate-400">لا توجد محافظ مجمدة حالياً</td></tr>';
          }
        }
      } catch (e) {
        console.error('Frozen wallets err:', e);
      }
    }

    async function toggleFraudRule(ruleId, isActive) {
      try {
        await apiFetch('/api/admin/fintech/fraud/rules/' + ruleId, {
          method: 'PUT',
          body: JSON.stringify({ is_active: isActive })
        });
        showToast('تم تحديث حالة قاعدة الحماية');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function unfreezeWallet(userId) {
      if (!confirm('هل أنت متأكد من فك تجميد هذه المحافظ؟ سيتم السماح للمستخدم باستئناف العمليات المالية.')) return;
      try {
        const res = await apiFetch('/api/admin/fintech/wallets/' + userId + '/unfreeze', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل فك التجميد');
        showToast('تم فك تجميد المحافظ بنجاح');
        loadFraudData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // FINTECH TAB 9: RECONCILIATION
    // =========================================================================
    async function loadReconciliationData() {
      try {
        const res = await apiFetch('/api/admin/stats');
        const data = await res.json();
        if (res.ok) {
          const zain = data.zaincash_volume_today || 0;
          const manual = data.manual_topups_today || 0;
          const balances = data.total_wallet_balance || 0;
          document.getElementById('rec-zaincash-total').innerText = formatIQD(zain);
          document.getElementById('rec-manual-total').innerText = formatIQD(manual);
          document.getElementById('rec-wallets-total').innerText = formatIQD(balances);
        }
      } catch (e) {
        console.error('Reconciliation data err:', e);
      }
    }

    async function loadReconciliationReports() {
      const tbody = document.getElementById('reconcile-reports-table-body');
      try {
        const res = await apiFetch('/api/admin/fintech/reconcile/reports');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-slate-400 font-bold">لا توجد تقارير تسوية سابقة</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(r => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold text-slate-600">#${r.id}</td>
              <td class="py-3 px-3 text-slate-800 font-bold">${formatBaghdad(r.period_start)}</td>
              <td class="py-3 px-3 font-black text-slate-800">${formatIQD(r.actual_received_iqd)}</td>
              <td class="py-3 px-3 font-black text-slate-800">${formatIQD(r.system_expected_iqd)}</td>
              <td class="py-3 px-3 font-black ${r.discrepancy_iqd === 0 ? 'text-emerald-600' : 'text-rose-600'}">${formatIQD(r.discrepancy_iqd)}</td>
              <td class="py-3 px-3">${r.discrepancy_iqd === 0 ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">مطابق تماماً</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600">يوجد فرق</span>'}</td>
              <td class="py-3 px-3 text-center">
                <button onclick="exportReconcileCSV(${r.id})" class="p-1.5 text-slate-500 hover:text-brand"><i class="fa-solid fa-download"></i></button>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل تقارير التسويات</td></tr>';
      }
    }

    async function runReconciliationNow() {
      const btn = document.getElementById('btn-run-reconcile');
      btn.disabled = true;
      btn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i><span>جاري احتساب التسوية والمطابقة...</span>';
      try {
        const res = await apiFetch('/api/admin/fintech/reconcile/run', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تشغيل التسوية');
        showToast('تمت المطابقة المحاسبية وحفظ التقرير بنجاح');
        loadReconciliationReports();
      } catch (err) {
        showToast(err.message, 'error');
      } finally {
        btn.disabled = false;
        btn.innerHTML = '<i class="fa-solid fa-play"></i><span>تشغيل التسويات الآن</span>';
      }
    }

    function exportReconcileCSV(reportId) {
      window.open('/api/admin/fintech/reconcile/reports/' + reportId + '/export?token=' + encodeURIComponent(authToken), '_blank');
    }

    // =========================================================================
    // FINTECH TAB 10: AGENT OVERDRAFT & DEBTS
    // =========================================================================
    async function loadAgentDebts() {
      const tbody = document.getElementById('agent-debts-table-body');
      try {
        const res = await apiFetch('/api/admin/fintech/wallets/debts');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-slate-400 font-bold">لا يوجد وكلاء مسجلون حالياً</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(a => {
            const avail = (a.wallet_balance || 0) + (a.credit_limit || 0);
            const debt = a.wallet_balance < 0 ? Math.abs(a.wallet_balance) : 0;
            return `
              <tr class="hover:bg-slate-50 transition">
                <td class="py-3 px-3">
                  <div class="font-bold text-slate-800">${escapeHtml(a.store_name || a.email)}</div>
                  <div class="text-[10px] text-slate-400 font-mono">${escapeHtml(a.email)}</div>
                </td>
                <td class="py-3 px-3 font-black ${a.wallet_balance < 0 ? 'text-rose-600' : 'text-emerald-600'}">${formatIQD(a.wallet_balance)}</td>
                <td class="py-3 px-3 font-black text-blue-600">${formatIQD(a.credit_limit || 0)}</td>
                <td class="py-3 px-3 font-black text-slate-800">${formatIQD(avail)}</td>
                <td class="py-3 px-3 font-black text-rose-600">${formatIQD(debt)}</td>
                <td class="py-3 px-3 text-center">
                  <button onclick="openSetCreditLimitModal(${a.id}, ${a.credit_limit || 0})" class="px-2.5 py-1 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-lg text-xs font-bold transition">تعديل السقف</button>
                </td>
              </tr>
            `;
          }).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل مديونية الوكلاء</td></tr>';
      }
    }

    function openSetCreditLimitModal(userId, currentLimit) {
      const select = document.getElementById('credit-limit-user-select');
      select.innerHTML = allUsers.filter(u => u.role === 'agent' || u.role === 'pos').map(u => `
        <option value="${u.id}" ${u.id === userId ? 'selected' : ''}>${escapeHtml(u.store_name || u.email)} (${formatIQD(u.wallet_balance || 0)})</option>
      `).join('');
      document.getElementById('credit-limit-amount').value = currentLimit || 0;
      openModal('modal-set-credit-limit');
    }

    async function handleSaveCreditLimit(e) {
      e.preventDefault();
      const userId = document.getElementById('credit-limit-user-select').value;
      const limit = parseInt(document.getElementById('credit-limit-amount').value, 10);
      try {
        const res = await apiFetch('/api/admin/fintech/wallets/' + userId + '/credit-limit', {
          method: 'POST',
          body: JSON.stringify({ credit_limit: limit })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تحديث الائتمان');
        closeModal('modal-set-credit-limit');
        showToast('تم تحديث الائتمان للوكيل بنجاح');
        loadAgentDebts();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // FINTECH TAB 11: FINANCIAL CLOSES & STATEMENTS
    // =========================================================================
    async function loadFinancialCloses() {
      const tbody = document.getElementById('closes-table-body');
      try {
        const res = await apiFetch('/api/admin/fintech/close/history');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-slate-400 font-bold">لا توجد إغلاقات مالية سابقة</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(c => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold text-slate-600">#${c.id}</td>
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(c.period_type === 'daily' ? 'إغلاق يومي' : 'إغلاق شهري')}</td>
              <td class="py-3 px-3 font-black text-slate-800">${formatIQD(c.opening_balance)}</td>
              <td class="py-3 px-3 font-black text-emerald-600">${formatIQD(c.total_credits)}</td>
              <td class="py-3 px-3 font-black text-rose-600">${formatIQD(c.total_debits)}</td>
              <td class="py-3 px-3 font-black text-slate-800">${formatIQD(c.closing_balance)}</td>
              <td class="py-3 px-3 font-mono text-[10px] text-slate-400 truncate max-w-xs" title="${c.seal_hash}">${c.seal_hash ? c.seal_hash.substring(0, 16) + '...' : '—'}</td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل الإغلاقات</td></tr>';
      }
    }

    async function sealFinancialCloseNow() {
      if (!confirm('هل أنت متأكد من ختم الإغلاق المالي لليوم؟ بعد الختم لا يمكن تعديل القيود السابقة.')) return;
      const btn = document.getElementById('btn-seal-close');
      btn.disabled = true;
      try {
        const res = await apiFetch('/api/admin/fintech/close/seal', {
          method: 'POST',
          body: JSON.stringify({ period_type: 'daily' })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل ختم الإغلاق المالي');
        showToast('تم ختم الإغلاق المالي بنجاح وحساب الهاش غير القابل للتعديل');
        loadFinancialCloses();
      } catch (err) {
        showToast(err.message, 'error');
      } finally {
        btn.disabled = false;
      }
    }

    function populateStatementUsers() {
      const sel = document.getElementById('statement-user-select');
      if (!sel) return;
      sel.innerHTML = '<option value="">-- اختر المشترك --</option>' + allUsers.map(u => `
        <option value="${u.id}">${escapeHtml(u.store_name || u.full_name)} (${escapeHtml(u.email)})</option>
      `).join('');
    }

    let lastStatementData = null;
    async function generateAccountStatement() {
      const userId = document.getElementById('statement-user-select').value;
      if (!userId) {
        showToast('يرجى اختيار المشترك المطلوب', 'error');
        return;
      }
      try {
        const res = await apiFetch('/api/admin/fintech/statements/' + userId);
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل توليد كشف الحساب');
        lastStatementData = data;

        document.getElementById('statement-header-info').innerText = 'المشترك: ' + (data.user_name || data.email) + ' | الرصيد الحالي: ' + formatIQD(data.current_balance);
        const body = document.getElementById('statement-content-body');
        const entries = data.entries || [];
        body.innerHTML = `
          <div class="p-4 bg-slate-50 rounded-2xl border border-slate-200 grid grid-cols-3 gap-2 text-center">
            <div><span class="text-slate-400 block text-[10px]">الرصيد الافتتاحي</span><span class="font-black text-slate-800">${formatIQD(data.opening_balance || 0)}</span></div>
            <div><span class="text-slate-400 block text-[10px]">مجموع الإيداعات</span><span class="font-black text-emerald-600">${formatIQD(data.total_credits || 0)}</span></div>
            <div><span class="text-slate-400 block text-[10px]">مجموع السحوبات</span><span class="font-black text-rose-600">${formatIQD(data.total_debits || 0)}</span></div>
          </div>
          <table class="w-full text-right border-collapse text-[11px] mt-2">
            <thead>
              <tr class="bg-slate-100 border-b border-slate-200 text-slate-600 font-bold">
                <th class="p-2">رقم القيد</th>
                <th class="p-2">المدين</th>
                <th class="p-2">الدائن</th>
                <th class="p-2">الرصيد بعدها</th>
                <th class="p-2">المرجع</th>
                <th class="p-2">التوقيت</th>
              </tr>
            </thead>
            <tbody>
              ${entries.map(e => `
                <tr class="border-b border-slate-100">
                  <td class="p-2 font-mono">#${e.id}</td>
                  <td class="p-2 font-bold text-rose-600">${e.debit > 0 ? formatIQD(e.debit) : '—'}</td>
                  <td class="p-2 font-bold text-emerald-600">${e.credit > 0 ? formatIQD(e.credit) : '—'}</td>
                  <td class="p-2 font-black text-slate-800">${formatIQD(e.balance_after)}</td>
                  <td class="p-2">${escapeHtml(e.ref_type)}</td>
                  <td class="p-2 text-slate-500">${formatBaghdad(e.created_at)}</td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        `;
        openModal('modal-statement');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function shareStatementWhatsApp() {
      if (!lastStatementData) return;
      const text = encodeURIComponent('كشف حساب رسمي من منصة VENUS POS للمشترك ' + (lastStatementData.user_name || '') + ' برصيد نهائي: ' + formatIQD(lastStatementData.current_balance));
      window.open('https://api.whatsapp.com/send?text=' + text, '_blank');
    }

    // =========================================================================
    // FINTECH TAB 12: PRICING TIERS & HIERARCHY
    // =========================================================================
    async function loadPricingData() {
      const grid = document.getElementById('pricing-tiers-grid');
      try {
        const res = await apiFetch('/api/admin/pricing/tiers');
        const tiers = await res.json();
        if (res.ok && Array.isArray(tiers)) {
          grid.innerHTML = tiers.map(t => `
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
              <div class="flex items-center justify-between">
                <h4 class="font-black text-sm text-slate-800">${escapeHtml(t.name_ar)}</h4>
                <span class="text-[10px] font-mono text-slate-400 font-bold">${escapeHtml(t.code)}</span>
              </div>
              <p class="text-xs text-slate-400">${escapeHtml(t.description || 'فئة تسعير معتمدة')}</p>
              <button onclick="openSetTierPriceModal(${t.id})" class="w-full py-2 bg-slate-50 hover:bg-brand hover:text-white text-slate-700 rounded-xl text-xs font-bold transition flex items-center justify-center gap-1.5">
                <i class="fa-solid fa-tags"></i><span>تسعير منتج لهذه الفئة</span>
              </button>
            </div>
          `).join('');

          // Update Add/Edit User Tier Select
          const selects = ['add-user-tier', 'edit-user-tier', 'tier-price-tier-select'];
          selects.forEach(sId => {
            const el = document.getElementById(sId);
            if (el) {
              el.innerHTML = tiers.map(t => '<option value="' + t.id + '">' + escapeHtml(t.name_ar) + '</option>').join('');
            }
          });
        }
      } catch (e) {
        console.error('Pricing tiers err:', e);
      }

      // Overrides
      const tbody = document.getElementById('subscriber-overrides-table-body');
      try {
        const res = await apiFetch('/api/admin/pricing/overrides');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" class="py-8 text-center text-slate-400 font-bold">لا توجد أسعار حصرية لمشتركين</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(o => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(o.store_name || o.email)}</td>
              <td class="py-3 px-3 font-mono font-bold text-slate-700">${escapeHtml(o.product_id)}</td>
              <td class="py-3 px-3 font-black text-emerald-600">${formatIQD(o.custom_price)}</td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatBaghdad(o.updated_at)}</td>
              <td class="py-3 px-3 text-center">
                <button onclick="deleteSubscriberOverride(${o.user_id}, '${escapeHtml(o.product_id)}')" class="px-2 py-1 bg-rose-50 text-rose-600 hover:bg-rose-100 rounded-lg text-xs font-bold transition">إلغاء الحصري</button>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="5" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل التخصيصات</td></tr>';
      }

      // Guard Settings (Stage 1)
      await loadGuardSettings();
      // Surge & Rewards & Restock (Stage 4)
      await loadSurgeConfig();
      await loadRewardRules();
      await loadRestockOrders();
    }

    async function loadGuardSettings() {
      const tbody = document.getElementById('guard-settings-table-body');
      if (!tbody) return;
      try {
        const res = await apiFetch('/api/admin/guard/settings');
        const data = await res.json();
        if (res.ok && Array.isArray(data)) {
          if (data.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-slate-400 font-bold">لا توجد قواعد هوامش مسجلة (الافتراضي: منع البيع بأقل من التكلفة فقط)</td></tr>';
            return;
          }
          tbody.innerHTML = data.map(g => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold ${g.item_id === '*' ? 'text-brand' : 'text-slate-800'}">${escapeHtml(g.item_id === '*' ? '* (الهامش العام لجميع المنتجات)' : g.item_id)}</td>
              <td class="py-3 px-3 font-black text-slate-800">${formatIQD(g.min_margin_iqd || 0)}</td>
              <td class="py-3 px-3 font-bold text-slate-700">${g.min_margin_pct || 0}%</td>
              <td class="py-3 px-3">${g.is_active ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">نشط</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-500">معطل</span>'}</td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatBaghdad(g.updated_at)}</td>
              <td class="py-3 px-3 text-center">
                <button onclick="deleteGuardSetting('${escapeHtml(g.item_id)}')" class="px-2 py-1 bg-rose-50 text-rose-600 hover:bg-rose-100 rounded-lg text-xs font-bold transition">حذف</button>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="6" class="py-8 text-center text-rose-500 font-bold">خطأ في تحميل إعدادات الحماية</td></tr>';
      }
    }

    function openAddGuardModal() {
      document.getElementById('guard-item-id').value = '';
      document.getElementById('guard-margin-iqd').value = '0';
      document.getElementById('guard-margin-pct').value = '0';
      document.getElementById('guard-is-active').checked = true;
      openModal('modal-add-guard');
    }

    async function submitAddGuardSetting(e) {
      e.preventDefault();
      const payload = {
        item_id: document.getElementById('guard-item-id').value.trim(),
        min_margin_iqd: parseInt(document.getElementById('guard-margin-iqd').value, 10) || 0,
        min_margin_pct: parseFloat(document.getElementById('guard-margin-pct').value) || 0,
        is_active: document.getElementById('guard-is-active').checked
      };
      try {
        const res = await apiFetch('/api/admin/guard/settings', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ إعدادات الهامش');
        closeModal('modal-add-guard');
        showToast('تم حفظ قاعدة الهامش الأدنى بنجاح');
        loadGuardSettings();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function deleteGuardSetting(itemId) {
      if (!confirm('هل أنت متأكد من حذف قاعدة الهامش الأدنى للصنف: ' + itemId + '؟')) return;
      try {
        const res = await apiFetch('/api/admin/guard/settings/' + encodeURIComponent(itemId), { method: 'DELETE' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حذف قاعدة الهامش');
        showToast('تم حذف قاعدة الهامش بنجاح');
        loadGuardSettings();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function openSetTierPriceModal(tierId) {
      if (tierId) document.getElementById('tier-price-tier-select').value = tierId;
      document.getElementById('tier-price-product-id').value = '';
      document.getElementById('tier-price-amount').value = '';
      openModal('modal-set-tier-price');
    }

    async function handleSaveTierPrice(e) {
      e.preventDefault();
      const tierId = document.getElementById('tier-price-tier-select').value;
      const productId = document.getElementById('tier-price-product-id').value.trim();
      const amount = parseInt(document.getElementById('tier-price-amount').value, 10);
      try {
        const res = await apiFetch('/api/admin/pricing/tiers/' + tierId + '/prices', {
          method: 'POST',
          body: JSON.stringify({ product_id: productId, price: amount })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ سعر الفئة');
        closeModal('modal-set-tier-price');
        showToast('تم حفظ سعر المنتج للفئة بنجاح');
        loadPricingData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    function openSetSubscriberOverrideModal() {
      const sel = document.getElementById('sub-override-user-select');
      sel.innerHTML = allUsers.map(u => '<option value="' + u.id + '">' + escapeHtml(u.store_name || u.email) + '</option>').join('');
      document.getElementById('sub-override-product-id').value = '';
      document.getElementById('sub-override-price').value = '';
      openModal('modal-set-subscriber-override');
    }

    async function handleSaveSubscriberOverride(e) {
      e.preventDefault();
      const userId = document.getElementById('sub-override-user-select').value;
      const productId = document.getElementById('sub-override-product-id').value.trim();
      const price = parseInt(document.getElementById('sub-override-price').value, 10);
      try {
        const res = await apiFetch('/api/admin/pricing/subscribers/' + userId + '/overrides', {
          method: 'POST',
          body: JSON.stringify({ product_id: productId, custom_price: price })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل تطبيق السعر الحصري');
        closeModal('modal-set-subscriber-override');
        showToast('تم تطبيق السعر الحصري للمشترك بنجاح');
        loadPricingData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function deleteSubscriberOverride(userId, productId) {
      if (!confirm('هل تريد إلغاء السعر الحصري لهذا المشترك؟')) return;
      try {
        const res = await apiFetch('/api/admin/pricing/subscribers/' + userId + '/overrides/' + encodeURIComponent(productId), { method: 'DELETE' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل إلغاء السعر');
        showToast('تم إلغاء السعر الحصري بنجاح');
        loadPricingData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function loadHierarchyData() {
      // placeholder for agent commissions view
    }

    function openSetAgentCommissionModal() {
      const sel = document.getElementById('agent-comm-user-select');
      sel.innerHTML = allUsers.filter(u => u.role === 'agent').map(u => '<option value="' + u.id + '">' + escapeHtml(u.store_name || u.email) + '</option>').join('');
      openModal('modal-set-agent-commission');
    }

    async function handleSaveAgentCommission(e) {
      e.preventDefault();
      const agentId = document.getElementById('agent-comm-user-select').value;
      const productId = document.getElementById('agent-comm-product-id').value.trim() || '*';
      const commType = document.getElementById('agent-comm-type').value;
      const commVal = parseFloat(document.getElementById('agent-comm-value').value);
      try {
        const res = await apiFetch('/api/admin/hierarchy/agents/' + agentId + '/commissions', {
          method: 'POST',
          body: JSON.stringify({ product_id: productId, commission_type: commType, commission_value: commVal })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ عمولة الوكيل');
        closeModal('modal-set-agent-commission');
        showToast('تم حفظ عمولة الوكيل وتسجيلها للشجرة');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // FINTECH TAB 13: VAULT RADAR & INVENTORY FORECASTING
    // =========================================================================
    async function loadRadarData() {
      const grid = document.getElementById('radar-categories-grid');
      try {
        const res = await apiFetch('/api/admin/radar/summary');
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل قراءة بيانات الرادار');
        updateRadarAlertUI(data);

        const cats = data.categories || [];
        if (cats.length === 0) {
          grid.innerHTML = '<div class="col-span-full py-12 text-center text-slate-400 font-bold">لا توجد بطاقات مسجلة في الخزنة</div>';
          return;
        }

        grid.innerHTML = cats.map(c => {
          const isLow = c.is_low_stock;
          return `
            <div class="bg-white rounded-3xl p-6 border ${isLow ? 'border-rose-300 shadow-rose-100' : 'border-slate-200/80'} shadow-sm space-y-4 flex flex-col justify-between">
              <div class="space-y-3">
                <div class="flex items-center justify-between">
                  <div>
                    <h4 class="font-black text-sm text-slate-800">${escapeHtml(c.category_name_ar)}</h4>
                    <span class="text-[10px] font-mono text-slate-400 font-bold">${escapeHtml(c.category_id)}</span>
                  </div>
                  ${isLow ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600 border border-rose-200">مخزون حرج</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600 border border-emerald-200">آمن</span>'}
                </div>
                <div class="p-3 bg-slate-50 rounded-2xl flex items-center justify-between">
                  <div><span class="text-[10px] text-slate-400 block font-bold">المتاح</span><span class="text-2xl font-black ${isLow ? 'text-rose-600' : 'text-slate-800'}">${c.available_count.toLocaleString()} كارت</span></div>
                  <div class="text-left"><span class="text-[10px] text-slate-400 block font-bold">الحد الأدنى</span><span class="text-xs font-bold text-slate-600">${c.min_threshold} كارت</span></div>
                </div>
                <div class="p-3 ${isLow ? 'bg-rose-50 border border-rose-200' : 'bg-slate-50'} rounded-2xl">
                  <div class="flex items-center justify-between text-xs font-bold mb-1">
                    <span class="${isLow ? 'text-rose-700' : 'text-slate-600'}">التنبؤ بنفاد الفئة:</span>
                    <span class="text-[10px] font-mono text-slate-400">معدل الحرق: ${c.burn_rate_per_hour}/ساعة</span>
                  </div>
                  <div class="text-xs font-black ${isLow ? 'text-rose-700' : 'text-brand'}">
                    <i class="fa-regular fa-clock ml-1"></i>${escapeHtml(c.exhaustion_text || 'طبيعي')}
                  </div>
                </div>
              </div>
              <button onclick="openSetCategoryThresholdModal('${escapeHtml(c.category_id)}', ${c.min_threshold})" class="w-full py-2 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl text-xs font-bold transition flex items-center justify-center gap-1.5">
                <i class="fa-solid fa-sliders"></i><span>تعديل حد الإنذار (${c.min_threshold})</span>
              </button>
            </div>
          `;
        }).join('');
      } catch (err) {
        grid.innerHTML = '<div class="col-span-full py-12 text-center text-rose-500 font-bold">خطأ في قراءة مؤشر النفاد: ' + err.message + '</div>';
      }
    }

    function updateRadarAlertUI(data) {
      const badgeNav = document.getElementById('badge-radar-low');
      if (data && data.has_low_stock) {
        if (badgeNav) badgeNav.classList.remove('hidden');
      } else {
        if (badgeNav) badgeNav.classList.add('hidden');
      }
    }

    async function checkRadarAlerts() {
      if (!authToken) return;
      try {
        const res = await apiFetch('/api/admin/radar/summary');
        const data = await res.json();
        if (res.ok) updateRadarAlertUI(data);
      } catch (e) {}
    }

    function openSetCategoryThresholdModal(catId, currentMin) {
      document.getElementById('threshold-cat-id').value = catId || '';
      document.getElementById('threshold-min-count').value = currentMin || 5;
      openModal('modal-set-category-threshold');
    }

    async function handleSaveCategoryThreshold(e) {
      e.preventDefault();
      const catId = document.getElementById('threshold-cat-id').value.trim();
      const minCount = parseInt(document.getElementById('threshold-min-count').value, 10);
      try {
        const res = await apiFetch('/api/admin/radar/thresholds', {
          method: 'POST',
          body: JSON.stringify({ category_id: catId, min_threshold: minCount })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ حد الإنذار');
        closeModal('modal-set-category-threshold');
        showToast('تم تحديث الحد الأدنى للإنذار بنجاح');
        loadRadarData();
        checkRadarAlerts();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    // =========================================================================
    // STAGE 4: SURGE PRICING, REWARDS RULES & AUTO-RESTOCK
    // =========================================================================
    async function loadSurgeConfig() {
      try {
        const res = await apiFetch('/api/admin/surge/config');
        const cfg = await res.json();
        if (res.ok) {
          const isEnabled = document.getElementById('surge-is-enabled');
          const pct = document.getElementById('surge-percentage');
          const maxCap = document.getElementById('surge-max-cap');
          const reason = document.getElementById('surge-reason');
          if (isEnabled) isEnabled.checked = cfg.is_enabled || false;
          if (pct) pct.value = cfg.surge_percentage || 0;
          if (maxCap) maxCap.value = cfg.max_surge_cap || 20;
          if (reason) reason.value = cfg.reason || '';
        }
      } catch (err) {
        console.error('loadSurgeConfig err:', err);
      }
    }

    async function saveSurgeConfig() {
      const payload = {
        is_enabled: document.getElementById('surge-is-enabled').checked,
        surge_percentage: parseFloat(document.getElementById('surge-percentage').value) || 0,
        max_surge_cap: parseFloat(document.getElementById('surge-max-cap').value) || 20,
        reason: document.getElementById('surge-reason').value.trim()
      };

      try {
        const res = await apiFetch('/api/admin/surge/config', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ إعدادات تسعير الذروة');
        showToast('تم حفظ إعدادات تسعير الذروة بنجاح');
        loadSurgeConfig();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function loadRewardRules() {
      const tbody = document.getElementById('reward-rules-table-body');
      if (!tbody) return;
      try {
        const res = await apiFetch('/api/admin/rewards/rules');
        const list = await res.json();
        if (res.ok && Array.isArray(list)) {
          if (list.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="py-6 text-center text-slate-400 font-bold">لا توجد قواعد مكافآت مسجلة</td></tr>';
            return;
          }
          tbody.innerHTML = list.map(r => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-bold text-slate-800">${escapeHtml(r.name)}</td>
              <td class="py-3 px-3 font-black text-slate-700">${r.metric === 'sales_amount' ? formatIQD(r.target_value) : r.target_value + ' عملية'}</td>
              <td class="py-3 px-3 font-black text-purple-600">${formatIQD(r.reward_amount)}</td>
              <td class="py-3 px-3"><span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-600">${r.period === 'monthly' ? 'شهري' : 'أسبوعي'}</span></td>
              <td class="py-3 px-3">${r.is_active ? '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">نشط</span>' : '<span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-500">معطل</span>'}</td>
              <td class="py-3 px-3 text-center">
                <button onclick="evaluateRewardForMerchant(${r.id})" class="px-2.5 py-1 bg-purple-50 text-purple-700 hover:bg-purple-100 rounded-lg text-xs font-bold transition flex items-center gap-1 mx-auto">
                  <i class="fa-solid fa-bolt text-[10px]"></i><span>صرف وتقييم</span>
                </button>
              </td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="6" class="py-6 text-center text-rose-500 font-bold">خطأ في تحميل قواعد المكافآت</td></tr>';
      }
    }

    function openAddRewardRuleModal() {
      document.getElementById('reward-rule-name').value = '';
      document.getElementById('reward-rule-target').value = '';
      document.getElementById('reward-rule-reward').value = '';
      document.getElementById('reward-rule-is-active').checked = true;
      openModal('modal-add-reward-rule');
    }

    async function submitAddRewardRule(e) {
      e.preventDefault();
      const payload = {
        name: document.getElementById('reward-rule-name').value.trim(),
        metric: document.getElementById('reward-rule-metric').value,
        period: document.getElementById('reward-rule-period').value,
        target_value: parseInt(document.getElementById('reward-rule-target').value, 10),
        reward_amount: parseInt(document.getElementById('reward-rule-reward').value, 10),
        is_active: document.getElementById('reward-rule-is-active').checked
      };

      try {
        const res = await apiFetch('/api/admin/rewards/rules', {
          method: 'POST',
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'فشل حفظ قاعدة المكافأة');
        closeModal('modal-add-reward-rule');
        showToast('تمت إضافة قاعدة المكافأة بنجاح 🎁');
        loadRewardRules();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function evaluateRewardForMerchant(ruleId) {
      const merchantIdStr = prompt('ادخل رقم المشترك / التاجر (Merchant ID) للتقييم والصرف:');
      if (!merchantIdStr) return;
      const merchantId = parseInt(merchantIdStr.trim(), 10);
      if (!merchantId) return;

      try {
        const res = await apiFetch('/api/admin/rewards/evaluate/' + merchantId + '/' + ruleId, { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'لم يتم استيفاء شروط المكافأة أو تم صرفها مسبقاً');
        showToast('تم تقييم وصرف المكافأة بنجاح في محفظة التاجر! 🎉');
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function loadRestockOrders() {
      const tbody = document.getElementById('restock-orders-table-body');
      if (!tbody) return;
      try {
        const res = await apiFetch('/api/admin/restock/orders');
        const list = await res.json();
        if (res.ok && Array.isArray(list)) {
          if (list.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="py-6 text-center text-slate-400 font-bold">لا توجد طلبات توريد سابقة</td></tr>';
            return;
          }
          tbody.innerHTML = list.map(o => `
            <tr class="hover:bg-slate-50 transition">
              <td class="py-3 px-3 font-mono font-bold text-slate-800">#${o.id}</td>
              <td class="py-3 px-3 font-bold text-blue-600 uppercase">${escapeHtml(o.provider)}</td>
              <td class="py-3 px-3 font-mono font-bold text-slate-700">${escapeHtml(o.item_id)}</td>
              <td class="py-3 px-3 font-black text-slate-800">${o.quantity}</td>
              <td class="py-3 px-3"><span class="px-2 py-0.5 rounded-full text-[10px] font-black ${o.status === 'completed' ? 'bg-emerald-50 text-emerald-600' : 'bg-amber-50 text-amber-700'}">${escapeHtml(o.status)}</span></td>
              <td class="py-3 px-3 text-slate-400 text-[11px]">${formatBaghdad(o.created_at)}</td>
            </tr>
          `).join('');
        }
      } catch (err) {
        tbody.innerHTML = '<tr><td colspan="6" class="py-6 text-center text-rose-500 font-bold">خطأ في تحميل طلبات التوريد</td></tr>';
      }
    }
