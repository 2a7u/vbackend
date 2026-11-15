package main

func GetTabsCore() string {
	return `        <!-- ========================================================================= -->
        <!-- TAB 1: DASHBOARD STATS                                                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-stats" class="tab-pane space-y-6">
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">إجمالي المشتركين</span>
              <div class="text-2xl font-black text-slate-800" id="stat-total-users">0</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">الاشتراكات الفعالة</span>
              <div class="text-2xl font-black text-brand" id="stat-active-subs">0</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">إجمالي أرصدة المحافظ</span>
              <div class="text-2xl font-black text-emerald-600" id="stat-total-balance">0 د.ع</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">حركات زين كاش اليوم</span>
              <div class="text-2xl font-black text-purple-600" id="stat-zaincash-today">0 د.ع</div>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 2: SUBSCRIBERS & POS MANAGEMENT (Phase 3 Enriched)                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-users" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">المشتركين</h2>
              <p class="text-xs text-slate-500 mt-0.5">البحث الفوري، ملف المشترك، معاينة، وتصدير البيانات</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button onclick="exportSubscribersCSV()" class="px-3.5 py-2 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 border border-emerald-200 rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-file-csv"></i><span>تصدير CSV</span>
              </button>
              <button onclick="openBulkActionModal()" class="px-3.5 py-2 bg-purple-50 hover:bg-purple-100 text-purple-700 border border-purple-200 rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-layer-group"></i><span>معاينة</span>
              </button>
              <button onclick="openAddUserModal()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-user-plus"></i><span>إضافة مشترك</span>
              </button>
            </div>
          </div>

          <!-- Filters Bar -->
          <div class="bg-white p-4 rounded-3xl border border-slate-200/80 shadow-sm flex flex-col md:flex-row items-center justify-between gap-3">
            <div class="w-full md:w-96 relative">
              <span class="absolute inset-y-0 right-0 flex items-center pr-3.5 text-slate-400"><i class="fa-solid fa-magnifying-glass text-xs"></i></span>
              <input type="text" id="user-search" oninput="renderUsersTable()" placeholder="ابحث بالاسم، المتجر، الهاتف، أو الإيميل..." class="w-full pr-10 pl-4 py-2 bg-slate-50 border border-slate-200 rounded-2xl text-xs font-bold text-slate-800 placeholder-slate-400 outline-none focus:border-brand focus:bg-white transition">
            </div>
            <div class="flex items-center gap-2 w-full md:w-auto overflow-x-auto pb-1 md:pb-0">
              <button onclick="setUserFilter('all')" class="user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold bg-brand text-white shadow-sm" data-filter="all">الكل</button>
              <button onclick="setUserFilter('subscriber')" class="user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold text-slate-600 hover:bg-slate-50" data-filter="subscriber">المشتركون</button>
              <button onclick="setUserFilter('pos')" class="user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold text-slate-600 hover:bg-slate-50" data-filter="pos">نقاط البيع</button>
              <button onclick="setUserFilter('agent')" class="user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold text-slate-600 hover:bg-slate-50" data-filter="agent">الوكلاء</button>
              <button onclick="setUserFilter('pending')" class="user-filter-btn px-3 py-1.5 rounded-xl text-xs font-bold text-slate-600 hover:bg-slate-50" data-filter="pending">بانتظار التفعيل</button>
            </div>
          </div>

          <!-- Users Table -->
          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">المشترك / المتجر</th>
                    <th class="py-3 px-3">الهاتف</th>
                    <th class="py-3 px-3">الدور</th>
                    <th class="py-3 px-3">الرصيد المالي (د.ع)</th>
                    <th class="py-3 px-3">الاشتراك</th>
                    <th class="py-3 px-3">الجهاز</th>
                    <th class="py-3 px-3">تاريخ الانضمام</th>
                    <th class="py-3 px-3 text-center">إجراءات Enterprise</th>
                  </tr>
                </thead>
                <tbody id="users-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="8" class="py-8 text-center text-slate-400">جاري تحميل قائمة المشتركين...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 3: ACTIVATION CODES & PLANS                                           -->
        <!-- ========================================================================= -->
        <section id="tab-content-activation" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الباقات</h2>
              <p class="text-xs text-slate-500 mt-0.5">توليد أكواد الاشتراك الفورية وإدارة الباقات السحابية</p>
            </div>
            <button onclick="openModal('modal-generate-codes')" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
              <i class="fa-solid fa-wand-magic-sparkles"></i><span>توليد دفعة أكواد جديدة</span>
            </button>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">سجل الباقات المصدرة</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">كود التفعيل</th>
                    <th class="py-3 px-3">الباقة</th>
                    <th class="py-3 px-3">المدة (أيام)</th>
                    <th class="py-3 px-3">الحالة</th>
                    <th class="py-3 px-3">المستخدم المفعل</th>
                    <th class="py-3 px-3">تاريخ التوليد</th>
                    <th class="py-3 px-3 text-center">نسخ</th>
                  </tr>
                </thead>
                <tbody id="codes-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="7" class="py-8 text-center text-slate-400">جاري تحميل الأكواد...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 4: WALLET & ZAINCASH                                                  -->
        <!-- ========================================================================= -->
        <section id="tab-content-wallet" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">المحافظ</h2>
              <p class="text-xs text-slate-500 mt-0.5">متابعة إيداعات المشتركين وشحن المحافظ وتأكيد دفعات بوابة ZainCash</p>
            </div>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">حركات بوابة الدفع الإلكتروني (ZainCash)</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رقم العملية (ID)</th>
                    <th class="py-3 px-3">المشترك</th>
                    <th class="py-3 px-3">المبلغ (د.ع)</th>
                    <th class="py-3 px-3">الحالة</th>
                    <th class="py-3 px-3">مرجع زين كاش</th>
                    <th class="py-3 px-3">التوقيت (بغداد)</th>
                  </tr>
                </thead>
                <tbody id="zaincash-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-8 text-center text-slate-400">جاري تحميل حركات زين كاش...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 5: CARDS & VOUCHER VAULT                                              -->
        <!-- ========================================================================= -->
        <section id="tab-content-cards" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">البطاقات</h2>
              <p class="text-xs text-slate-500 mt-0.5">البطاقات الرقمية المشفرة، منع التكرار ببصمة HMAC، ورفع الكروت بالدفعات</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="openModal('modal-add-card')" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-plus"></i><span>إضافة فئة بطاقة جديدة</span>
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" id="cards-grid">
            <div class="col-span-full py-12 text-center text-slate-400">جاري تحميل باقات الخزنة...</div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 6: FORENSIC AUDIT TRAIL                                               -->
        <!-- ========================================================================= -->
        <section id="tab-content-audit" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">سجل التدقيق</h2>
              <p class="text-xs text-slate-500 mt-0.5">توثيق كامل للعمليات الإدارية والمالية مع إمكانية عكس العملية</p>
            </div>
            <button onclick="loadAuditLogs()" class="px-3.5 py-2 bg-white hover:bg-slate-50 border border-slate-200 text-slate-700 rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
              <i class="fa-solid fa-rotate-right text-brand"></i><span>تحديث السجل</span>
            </button>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">رقم السجل</th>
                    <th class="py-3 px-3">المنفذ</th>
                    <th class="py-3 px-3">الإجراء</th>
                    <th class="py-3 px-3">الهدف</th>
                    <th class="py-3 px-3">التفاصيل الفنية</th>
                    <th class="py-3 px-3">التوقيت (بغداد)</th>
                    <th class="py-3 px-3 text-center">إجراء عكسي</th>
                  </tr>
                </thead>
                <tbody id="audit-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="7" class="py-8 text-center text-slate-400">جاري تحميل سجل التدقيق...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>
`
}
