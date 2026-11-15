package main

func GetTabsEnterprise() string {
	return `        <!-- ========================================================================= -->
        <!-- TAB 14: PROMOTIONS & COUPONS (Phase 4)                                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-coupons" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الخصومات</h2>
              <p class="text-xs text-slate-500 mt-0.5">إدارة أكواد الخصم، العمولات التلقائية للمناديب، ونسب الخصم</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="openAddCouponModal()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-plus"></i>
                <span>إنشاء كود خصم جديد</span>
              </button>
            </div>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">كود الخصم</th>
                    <th class="py-3 px-3">نوع وقيمة الخصم</th>
                    <th class="py-3 px-3">الاستخدامات / الحد الأقصى</th>
                    <th class="py-3 px-3">المندوب المرتبط</th>
                    <th class="py-3 px-3">عمولة المندوب (د.ع)</th>
                    <th class="py-3 px-3">الحالة والصلاحية</th>
                  </tr>
                </thead>
                <tbody id="coupons-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-8 text-center text-slate-400">جاري تحميل الخصومات...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 15: REMOTE APP CONFIG (Phase 5)                                       -->
        <!-- ========================================================================= -->
        <section id="tab-content-appconfig" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">إعدادات المنصة</h2>
              <p class="text-xs text-slate-500 mt-0.5">الصيانة، إجبار التحديث، Feature Flags، البنرات وشريط الأخبار</p>
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <!-- Maintenance Mode & Kill-Switch -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <div class="w-8 h-8 rounded-xl bg-amber-50 text-amber-600 flex items-center justify-center text-sm font-black"><i class="fa-solid fa-power-off"></i></div>
                  <div>
                    <h3 class="text-sm font-black text-slate-800">الصيانة</h3>
                    <p class="text-[11px] text-slate-400">تعطيل وصول المشتركين فورياً مع استثناء المدراء</p>
                  </div>
                </div>
                <input type="checkbox" id="cfg-maintenance-toggle" class="w-5 h-5 accent-brand cursor-pointer">
              </div>
              <div>
                <label class="text-[11px] font-bold text-slate-500 block mb-1">رسالة الصيانة المعروضة بالتطبيق:</label>
                <input type="text" id="cfg-maintenance-msg" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-medium outline-none focus:border-brand">
              </div>
              <button onclick="saveMaintenanceConfig()" class="w-full py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">حفظ الصيانة</button>
            </div>

            <!-- Force Update -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <div class="w-8 h-8 rounded-xl bg-blue-50 text-brand flex items-center justify-center text-sm font-black"><i class="fa-solid fa-cloud-arrow-up"></i></div>
                  <div>
                    <h3 class="text-sm font-black text-slate-800">إجبار التحديث (Force Update)</h3>
                    <p class="text-[11px] text-slate-400">منع النسخ القديمة من الاتصال بالسيرفر</p>
                  </div>
                </div>
                <input type="checkbox" id="cfg-force-update-toggle" class="w-5 h-5 accent-brand cursor-pointer">
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div>
                  <label class="text-[11px] font-bold text-slate-500 block mb-1">الحد الأدنى للإصدار:</label>
                  <input type="text" id="cfg-min-version" placeholder="1.0.0" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-medium outline-none focus:border-brand">
                </div>
                <div>
                  <label class="text-[11px] font-bold text-slate-500 block mb-1">رابط التحميل المباشر:</label>
                  <input type="text" id="cfg-update-url" placeholder="https://..." class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-medium outline-none focus:border-brand">
                </div>
              </div>
              <button onclick="saveForceUpdateConfig()" class="w-full py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">حفظ إعدادات التحديث</button>
            </div>

            <!-- Feature Flags -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4 md:col-span-2">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-toggle-on text-brand"></i>
                <span>الخدمات</span>
              </h3>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-xs font-bold text-slate-700">
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-zaincash" class="accent-brand w-4 h-4"><span>بوابة زين كاش</span>
                </label>
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-vouchers" class="accent-brand w-4 h-4"><span>بيع البطاقات</span>
                </label>
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-trial" class="accent-brand w-4 h-4"><span>التجربة المجانية</span>
                </label>
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-debts" class="accent-brand w-4 h-4"><span>الديون والآجل</span>
                </label>
              </div>
              <button onclick="saveFeatureFlagsConfig()" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">حفظ الـ Feature Flags</button>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 16: LIVE DEVICE RADAR & COMMANDS (Phase 6)                            -->
        <!-- ========================================================================= -->
        <section id="tab-content-deviceradar" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الأجهزة المتصلة</h2>
              <p class="text-xs text-slate-500 mt-0.5">مراقبة اتصال الأجهزة لحظياً، سرعة الاستجابة، وإرسال أوامر القائمة البيضاء المعتمدة</p>
            </div>
            <button onclick="loadDeviceRadarData()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-2 transition">
              <i class="fa-solid fa-rotate-right"></i>
              <span>تحديث الرادار الحي</span>
            </button>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">الأجهزة المتصلة أونلاين الآن عبر WebSocket</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">المشترك / المتجر</th>
                    <th class="py-3 px-3">موديل الهاتف</th>
                    <th class="py-3 px-3">إصدار التطبيق</th>
                    <th class="py-3 px-3">زمن الاستجابة (Latency)</th>
                    <th class="py-3 px-3">الشاشة الحالية</th>
                    <th class="py-3 px-3">آخر نشاط</th>
                    <th class="py-3 px-3 text-center">أوامر عن بعد</th>
                  </tr>
                </thead>
                <tbody id="online-devices-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="7" class="py-8 text-center text-slate-400">جاري مسح الأجهزة المتصلة...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 17: SECURITY, WAF & API KEYS (Phase 7)                                -->
        <!-- ========================================================================= -->
        <section id="tab-content-security" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الرقابة والأمان</h2>
              <p class="text-xs text-slate-500 mt-0.5">قوائم حظر العناوين IP، كشف محاولات الاختراق، وتوليد مفاتيح API المشفرة</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="openAddIPRuleModal()" class="px-3.5 py-2 bg-rose-50 hover:bg-rose-100 text-rose-700 border border-rose-200 rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-ban"></i><span>إضافة قاعدة IP</span>
              </button>
              <button onclick="openAddAPIKeyModal()" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-key"></i><span>توليد مفتاح API جديد</span>
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <!-- IP Rules Table -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-shield-halved text-rose-600"></i>
                <span>قواعد جدار الحماية (IP Rules & Bans)</span>
              </h3>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">عنوان IP</th>
                      <th class="py-2.5 px-3">النوع</th>
                      <th class="py-2.5 px-3">السبب</th>
                      <th class="py-2.5 px-3 text-center">إجراء</th>
                    </tr>
                  </thead>
                  <tbody id="ip-rules-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="4" class="py-6 text-center text-slate-400">جاري تحميل قواعد الحظر...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- API Keys Table -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-key text-brand"></i>
                <span>مفاتيح API الخارجية المعتمدة</span>
              </h3>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">اسم المفتاح</th>
                      <th class="py-2.5 px-3">البادئة</th>
                      <th class="py-2.5 px-3">الدور</th>
                      <th class="py-2.5 px-3">الحالة</th>
                    </tr>
                  </thead>
                  <tbody id="api-keys-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="4" class="py-6 text-center text-slate-400">جاري تحميل مفاتيح API...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 18: BUSINESS INTELLIGENCE & CLOUD BACKUP (Phase 8)                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-bi" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">التقارير</h2>
              <p class="text-xs text-slate-500 mt-0.5">تحليلات الأرباح الصافية، معدلات الاحتفاظ والانسحاب، والنسخ السريع للبيانات</p>
            </div>
            <button onclick="downloadSecureBackup()" class="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl font-bold text-xs shadow-md shadow-emerald-600/20 flex items-center gap-2 transition">
              <i class="fa-solid fa-download"></i>
              <span>تنزيل نسخة احتياطية كاملة (JSON مشفر)</span>
            </button>
          </div>

          <!-- BI KPI Cards -->
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">صافي أرباح البطاقات</span>
              <div class="text-xl font-black text-emerald-600" id="bi-vouchers-profit">0 د.ع</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">إيراد الاشتراكات الإلكترونية</span>
              <div class="text-xl font-black text-brand" id="bi-sub-profit">0 د.ع</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">معدل الاحتفاظ (Retention)</span>
              <div class="text-xl font-black text-purple-600" id="bi-retention-rate">0%</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">معدل الانسحاب (Churn)</span>
              <div class="text-xl font-black text-rose-600" id="bi-churn-rate">0%</div>
            </div>
          </div>

          <!-- Live Server Health Box -->
          <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
            <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
              <i class="fa-solid fa-server text-emerald-600"></i>
              <span>مؤشرات أداء السيرفر المباشرة (Live Telemetry)</span>
            </h3>
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-xs font-bold text-slate-700">
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">زمن استجابة DB</span>
                <span id="bi-db-latency" class="text-sm font-black text-emerald-600">0 ms</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">الـ Goroutines النشطة</span>
                <span id="bi-goroutines" class="text-sm font-black text-brand">0</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">استهلاك RAM المستخدم</span>
                <span id="bi-ram" class="text-sm font-black text-slate-800">0 MB</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">توقيت السيرفر الرسمي</span>
                <span id="bi-server-time" class="text-xs font-bold text-slate-600">—</span>
              </div>
            </div>
          </div>
        </section>
`
}
