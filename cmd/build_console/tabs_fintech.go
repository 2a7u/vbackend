package main

func GetTabsFintech() string {
	return `        <!-- ========================================================================= -->
        <!-- TAB 7: DOUBLE-ENTRY IMMUTABLE LEDGER (Phase 1.1)                           -->
        <!-- ========================================================================= -->
        <section id="tab-content-ledger" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-xl font-black text-slate-800">السجل المالي</h2>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand border border-blue-200">
                  <i class="fa-solid fa-lock text-[9px] ml-1"></i>سلاسل SHA-256
                </span>
              </div>
              <p class="text-xs text-slate-500 mt-0.5">سجل محاسبي غير قابل للتعديل مشفر بسلاسل التجزئة مع التحقق الجنائي اللحظي</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="triggerIntegrityCheck()" id="btn-verify-integrity" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-fingerprint"></i>
                <span>بدء الفحص الجنائي لسلاسل الهاش</span>
              </button>
              <button onclick="loadLedgerEntries()" class="px-3.5 py-2 bg-white hover:bg-slate-50 border border-slate-200 text-slate-700 rounded-xl font-bold text-xs shadow-sm flex items-center gap-2 transition">
                <i class="fa-solid fa-rotate-right text-brand"></i>
                <span>تحديث القيود</span>
              </button>
            </div>
          </div>

          <!-- Integrity Status Card -->
          <div id="integrity-status-card" class="p-4 rounded-3xl bg-emerald-50/80 border border-emerald-200 flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="w-12 h-12 rounded-2xl bg-emerald-500 text-white flex items-center justify-center text-xl shadow-md shadow-emerald-500/20 flex-shrink-0">
                <i class="fa-solid fa-shield-check"></i>
              </div>
              <div>
                <h3 class="font-black text-sm text-emerald-900" id="integrity-status-title">سلاسل السجل المالي سليمة 100% ومطابقة محاسبياً</h3>
                <p class="text-xs text-emerald-700 mt-0.5" id="integrity-status-desc">تم التحقق من كافة القيود، وتطابق الأرصدة التراكمية، ولا توجد أي انقطاعات أو تلاعب بقواعد البيانات.</p>
              </div>
            </div>
            <div class="text-left flex-shrink-0">
              <span class="text-[10px] text-slate-400 block font-bold">آخر فحص جنائي</span>
              <span class="text-xs font-mono font-bold text-slate-700" id="integrity-last-checked">—</span>
            </div>
          </div>

          <!-- Ledger Table -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رقم القيد / الحركة</th>
                    <th class="py-3 px-3">الحساب المالي</th>
                    <th class="py-3 px-3">المدين (Debit)</th>
                    <th class="py-3 px-3">الدائن (Credit)</th>
                    <th class="py-3 px-3">الرصيد بعد القيد</th>
                    <th class="py-3 px-3">نوع المرجع</th>
                    <th class="py-3 px-3">التاريخ والوقت (بغداد)</th>
                    <th class="py-3 px-3">ختم الهاش SHA-256</th>
                  </tr>
                </thead>
                <tbody id="ledger-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="8" class="py-8 text-center text-slate-400">جاري تحميل قيود السجل المالي...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 8: REAL-TIME FRAUD ENGINE & AUTO-FREEZE (Phase 1.2)                   -->
        <!-- ========================================================================= -->
        <section id="tab-content-fraud" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-xl font-black text-slate-800">الحماية</h2>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600 border border-rose-200">
                  <i class="fa-solid fa-bolt text-[9px] ml-1"></i>Auto-Freeze لحظي
                </span>
              </div>
              <p class="text-xs text-slate-500 mt-0.5">قواعد انزلاقية Sliding Window، تجميد فوري للأنشطة المشبوهة، وإدارة فك التجميد الإداري</p>
            </div>
            <button onclick="loadFraudData()" class="px-3.5 py-2 bg-white hover:bg-slate-50 border border-slate-200 text-slate-700 rounded-xl font-bold text-xs shadow-sm flex items-center gap-2 transition">
              <i class="fa-solid fa-rotate-right text-brand"></i>
              <span>تحديث بيانات الاحتيال</span>
            </button>
          </div>

          <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <!-- Fraud Rules Config Card -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-sliders text-brand"></i>
                <span>قواعد الرصد النشطة في النظام</span>
              </h3>
              <div class="space-y-3" id="fraud-rules-list">
                <div class="py-6 text-center text-slate-400 text-xs">جاري تحميل قواعد الكشف...</div>
              </div>
            </div>

            <!-- Frozen Wallets Card -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
              <div class="flex items-center justify-between">
                <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                  <i class="fa-solid fa-snowflake text-rose-600"></i>
                  <span>المحافظ المجمدة حالياً احترازياً</span>
                </h3>
                <span id="frozen-count-badge" class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600">0 حساب</span>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">المشترك / المتجر</th>
                      <th class="py-2.5 px-3">سبب التجميد</th>
                      <th class="py-2.5 px-3 text-center">فك التجميد</th>
                    </tr>
                  </thead>
                  <tbody id="frozen-wallets-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="3" class="py-6 text-center text-slate-400">لا توجد محافظ مجمدة حالياً</td></tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 9: RECONCILIATION & AUDIT BALANCE (Phase 1.3)                         -->
        <!-- ========================================================================= -->
        <section id="tab-content-reconcile" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-xl font-black text-slate-800">التسويات</h2>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand border border-blue-200">
                  <i class="fa-solid fa-equals text-[9px] ml-1"></i>المعادلة المحاسبية الصفرية
                </span>
              </div>
              <p class="text-xs text-slate-500 mt-0.5">مطابقة المبالغ الفعلية المستلمة مع أرصدة المحافظ وقيم الكروت المباعة مع تصدير التقارير</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="runReconciliationNow()" id="btn-run-reconcile" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-play"></i>
                <span>تشغيل التسويات الآن</span>
              </button>
            </div>
          </div>

          <!-- Reconciliation Formula Card -->
          <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
            <h3 class="text-xs font-black text-slate-400 uppercase tracking-wider">مؤشرات التسوية اللحظية للمنصة</h3>
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
              <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-100">
                <span class="text-[10px] text-slate-400 block font-bold">إجمالي زين كاش المستلم</span>
                <span id="rec-zaincash-total" class="text-lg font-black text-slate-800">0 د.ع</span>
              </div>
              <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-100">
                <span class="text-[10px] text-slate-400 block font-bold">إجمالي الشحن اليدوي</span>
                <span id="rec-manual-total" class="text-lg font-black text-slate-800">0 د.ع</span>
              </div>
              <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-100">
                <span class="text-[10px] text-slate-400 block font-bold">مجموع المحافظ + الكروت</span>
                <span id="rec-wallets-total" class="text-lg font-black text-slate-800">0 د.ع</span>
              </div>
              <div class="p-3.5 rounded-2xl border bg-emerald-50 border-emerald-200" id="rec-diff-box">
                <span class="text-[10px] text-emerald-800 block font-bold">فرق المطابقة (Discrepancy)</span>
                <span id="rec-diff-value" class="text-lg font-black text-emerald-700">0 د.ع (مطابق تماماً)</span>
              </div>
            </div>
          </div>

          <!-- Past Reconciliation Reports Table -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">سجل تقارير التسويات</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رقم التقرير</th>
                    <th class="py-3 px-3">الفترة والمطابقة</th>
                    <th class="py-3 px-3">المستلم الفعلي</th>
                    <th class="py-3 px-3">المحسوب بالنظام</th>
                    <th class="py-3 px-3">الفرق المالي</th>
                    <th class="py-3 px-3">الحالة المحاسبية</th>
                    <th class="py-3 px-3 text-center">تصدير CSV</th>
                  </tr>
                </thead>
                <tbody id="reconcile-reports-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="7" class="py-8 text-center text-slate-400">جاري تحميل تقارير التسويات...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 10: AGENT OVERDRAFT & CREDIT LIMITS (Phase 1.4)                       -->
        <!-- ========================================================================= -->
        <section id="tab-content-debts" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-xl font-black text-slate-800">الائتمان</h2>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand border border-blue-200">
                  <i class="fa-solid fa-lock text-[9px] ml-1"></i>Row-Lock آمن ضد Race Conditions
                </span>
              </div>
              <p class="text-xs text-slate-500 mt-0.5">تحديد الائتمان المسموح به لكل وكيل، والتحكم بالأرصدة السالبة بأمان تام</p>
            </div>
            <button onclick="openSetCreditLimitModal()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
              <i class="fa-solid fa-pen-ruler"></i>
              <span>تعديل الائتمان</span>
            </button>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">قائمة الوكلاء والائتمان والمديونية الحالية</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">الوكيل / المتجر</th>
                    <th class="py-3 px-3">الرصيد الفعلي</th>
                    <th class="py-3 px-3">الائتمان</th>
                    <th class="py-3 px-3">الرصيد المتاح للسحب</th>
                    <th class="py-3 px-3">المديونية القائمة</th>
                    <th class="py-3 px-3 text-center">إجراء</th>
                  </tr>
                </thead>
                <tbody id="agent-debts-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-8 text-center text-slate-400">جاري تحميل مديونية الوكلاء...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 11: FINANCIAL CLOSES & STATEMENTS (Phase 1.5)                         -->
        <!-- ========================================================================= -->
        <section id="tab-content-close" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-xl font-black text-slate-800">الإغلاق المالي</h2>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand border border-blue-200">
                  <i class="fa-solid fa-stamp text-[9px] ml-1"></i>ختم رقمي مشفر
                </span>
              </div>
              <p class="text-xs text-slate-500 mt-0.5">الإغلاقات المحاسبية اليومية والشهرية المختومة، وإصدار كشوفات الحساب الرسمية</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="sealFinancialCloseNow()" id="btn-seal-close" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-stamp"></i>
                <span>ختم الإغلاق المالي لليوم</span>
              </button>
            </div>
          </div>

          <!-- Generate Statement Card -->
          <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
            <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
              <i class="fa-solid fa-file-invoice text-brand"></i>
              <span>إصدار كشف حساب تفصيلي لمشترك أو نقطة بيع</span>
            </h3>
            <div class="grid grid-cols-1 sm:grid-cols-4 gap-3 items-end">
              <div>
                <label class="block text-xs font-bold text-slate-700 mb-1">المشترك / الوكيل</label>
                <select id="statement-user-select" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
                  <option value="">-- اختر المشترك --</option>
                </select>
              </div>
              <div>
                <label class="block text-xs font-bold text-slate-700 mb-1">من تاريخ</label>
                <input type="date" id="statement-from-date" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              </div>
              <div>
                <label class="block text-xs font-bold text-slate-700 mb-1">إلى تاريخ</label>
                <input type="date" id="statement-to-date" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              </div>
              <button onclick="generateAccountStatement()" class="w-full py-2 bg-slate-800 hover:bg-slate-900 text-white rounded-xl text-xs font-bold transition flex items-center justify-center gap-1.5 shadow-sm">
                <i class="fa-solid fa-magnifying-glass"></i><span>عرض كشف الحساب</span>
              </button>
            </div>
          </div>

          <!-- Closed Batches History Table -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">سجل الإغلاقات المالية المختومة تاريخياً</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رقم الإغلاق</th>
                    <th class="py-3 px-3">نوع الفترة</th>
                    <th class="py-3 px-3">الرصيد الافتتاحي</th>
                    <th class="py-3 px-3">الإيداعات</th>
                    <th class="py-3 px-3">السحوبات</th>
                    <th class="py-3 px-3">الرصيد الختامي</th>
                    <th class="py-3 px-3">الختم الرقمي (Seal Hash)</th>
                  </tr>
                </thead>
                <tbody id="closes-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="7" class="py-8 text-center text-slate-400">جاري تحميل الإغلاقات المالية...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 12: PRICING TIERS & COMMISSIONS (Phase 2.2, 2.3)                      -->
        <!-- ========================================================================= -->
        <section id="tab-content-pricing" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الأسعار</h2>
              <p class="text-xs text-slate-500 mt-0.5">أسعار الفئات (VIP، جملة، عادي)، الأسعار الحصرية، وشجرة عمولات الوكلاء</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="openSetSubscriberOverrideModal()" class="px-3.5 py-2 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 border border-emerald-200 rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-star"></i><span>تخصيص سعر لمشترك</span>
              </button>
              <button onclick="openSetAgentCommissionModal()" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-1.5 transition">
                <i class="fa-solid fa-sitemap"></i><span>ضبط عمولة وكيل بالشجرة</span>
              </button>
            </div>
          </div>

          <!-- Pricing Tiers Grid -->
          <div class="space-y-3">
            <h3 class="text-sm font-black text-slate-800">فئات الأسعار (VIP / Wholesale / POS)</h3>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4" id="pricing-tiers-grid">
              <div class="col-span-full py-8 text-center text-slate-400">جاري تحميل فئات الأسعار...</div>
            </div>
          </div>

          <!-- Subscriber Overrides Table -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">الأسعار الحصرية المخصصة لمشتركين معينين (Overrides)</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">المشترك / المتجر</th>
                    <th class="py-3 px-3">رمز المنتج</th>
                    <th class="py-3 px-3">السعر الحصري (د.ع)</th>
                    <th class="py-3 px-3">تاريخ التحديث</th>
                    <th class="py-3 px-3 text-center">إجراء</th>
                  </tr>
                </thead>
                <tbody id="subscriber-overrides-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="5" class="py-8 text-center text-slate-400">جاري تحميل التخصيصات...</td></tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Guard Minimum Margin Settings (Stage 1) -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-5 space-y-3">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                  <i class="fa-solid fa-shield-halved text-brand"></i>
                  <span>حماية الأسعار وهامش الربح الأدنى (Guard)</span>
                </h3>
                <p class="text-[11px] text-slate-400">منع البيع بأقل من التكلفة وإلزام هامش ربح أدنى لكل صنف أو للكل (*)</p>
              </div>
              <button onclick="openAddGuardModal()" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-plus"></i><span>إضافة هامش أدنى</span>
              </button>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رمز الصنف / المنتج</th>
                    <th class="py-3 px-3">الهامش الأدنى (د.ع)</th>
                    <th class="py-3 px-3">الهامش الأدنى (%)</th>
                    <th class="py-3 px-3">الحالة</th>
                    <th class="py-3 px-3">آخر تحديث</th>
                    <th class="py-3 px-3 text-center">إجراء</th>
                  </tr>
                </thead>
                <tbody id="guard-settings-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-6 text-center text-slate-400">جاري تحميل إعدادات الحماية...</td></tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Surge Dynamic Pricing (Stage 4) -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-5 space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                  <i class="fa-solid fa-arrow-trend-up text-amber-500"></i>
                  <span>تسعير الذروة الديناميكي (Surge Pricing)</span>
                </h3>
                <p class="text-[11px] text-slate-400">معطل افتراضياً، مع تحديد سقف أقصى للزيادة وعرض النسبة والسبب للمشتري قبل الدفع</p>
              </div>
              <button onclick="saveSurgeConfig()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">
                <i class="fa-solid fa-check ml-1"></i>حفظ إعدادات الذروة
              </button>
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-4 gap-4 text-xs font-bold text-slate-700 bg-slate-50 p-4 rounded-2xl border border-slate-200/80">
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" id="surge-is-enabled" class="accent-brand w-4 h-4">
                <span>تفعيل تسعير الذروة</span>
              </label>
              <div>
                <label class="block text-[11px] text-slate-500 mb-1">نسبة الزيادة الحالية (%)</label>
                <input type="number" id="surge-percentage" min="0" max="100" class="w-full px-3 py-1.5 bg-white border border-slate-200 rounded-xl text-xs font-bold">
              </div>
              <div>
                <label class="block text-[11px] text-slate-500 mb-1">السقف الأقصى المسموح (%)</label>
                <input type="number" id="surge-max-cap" min="0" max="100" class="w-full px-3 py-1.5 bg-white border border-slate-200 rounded-xl text-xs font-bold">
              </div>
              <div>
                <label class="block text-[11px] text-slate-500 mb-1">سبب الذروة للمشتري</label>
                <input type="text" id="surge-reason" placeholder="مثال: ذروة الطلب بداية الشهر" class="w-full px-3 py-1.5 bg-white border border-slate-200 rounded-xl text-xs font-bold">
              </div>
            </div>
          </div>

          <!-- Sales Rewards Rules (Stage 4) -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-5 space-y-3">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                  <i class="fa-solid fa-gift text-purple-600"></i>
                  <span>برنامج مكافآت وحوافز المبيعات (Sales Rewards)</span>
                </h3>
                <p class="text-[11px] text-slate-400">صرف مكافآت آلية لمحفظة التاجر عند تجاوز مستهدف المبيعات شهرياً أو أسبوعياً</p>
              </div>
              <div class="flex items-center gap-2">
                <button onclick="openAddRewardRuleModal()" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                  <i class="fa-solid fa-plus"></i><span>إضافة قاعدة مكافأة</span>
                </button>
                <button onclick="loadRewardRules()" class="px-3 py-2 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition">
                  <i class="fa-solid fa-rotate-right"></i>
                </button>
              </div>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">اسم القاعدة</th>
                    <th class="py-3 px-3">المستهدف (Target)</th>
                    <th class="py-3 px-3">المكافأة (د.ع)</th>
                    <th class="py-3 px-3">الفترة</th>
                    <th class="py-3 px-3">الحالة</th>
                    <th class="py-3 px-3 text-center">إجراء</th>
                  </tr>
                </thead>
                <tbody id="reward-rules-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-6 text-center text-slate-400">جاري تحميل قواعد المكافآت...</td></tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Restock Orders (Stage 4) -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-5 space-y-3">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                  <i class="fa-solid fa-boxes-packing text-blue-600"></i>
                  <span>طلبات إعادة التعبئة والتوريد الآلي (Auto-Restock)</span>
                </h3>
                <p class="text-[11px] text-slate-400">سجل طلبات التوريد التلقائية من المزودين الخارجيين عند اقتراب نفاد المخزون</p>
              </div>
              <button onclick="loadRestockOrders()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition">
                <i class="fa-solid fa-rotate-right"></i><span>تحديث</span>
              </button>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رقم الطلب</th>
                    <th class="py-3 px-3">المزود</th>
                    <th class="py-3 px-3">رمز الصنف</th>
                    <th class="py-3 px-3">الكمية</th>
                    <th class="py-3 px-3">الحالة</th>
                    <th class="py-3 px-3">التاريخ</th>
                  </tr>
                </thead>
                <tbody id="restock-orders-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-6 text-center text-slate-400">جاري تحميل طلبات التوريد...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 13: VAULT RADAR & INVENTORY FORECASTING (Phase 2.4)                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-radar" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h2 class="text-xl font-black text-slate-800">مؤشر النفاد</h2>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand border border-blue-200">
                  <i class="fa-solid fa-fire text-[9px] ml-1"></i>معدل الحرق (Burn Rate)
                </span>
              </div>
              <p class="text-xs text-slate-500 mt-0.5">مراقبة استهلاك كروت الخزنة اللحظي والتنبؤ بموعد نفاد كل فئة لمنع توقف نقاط البيع</p>
            </div>
            <button onclick="loadRadarData()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
              <i class="fa-solid fa-rotate-right"></i>
              <span>تحديث بيانات الرادار</span>
            </button>
          </div>

          <!-- Radar Categories Grid -->
          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6" id="radar-categories-grid">
            <div class="col-span-full py-12 text-center text-slate-400 font-bold">جاري مسح مخزون الخزنة وحساب معدلات الاستهلاك...</div>
          </div>
        </section>
`
}
