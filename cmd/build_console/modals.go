package main

func GetModalsCore() string {
	return `  <!-- 1. ADD USER MODAL -->
  <div id="modal-add-user" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">إضافة مشترك</h3>
        <button onclick="closeModal('modal-add-user')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleCreateUser(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الاسم الكامل</label>
          <input type="text" id="add-user-name" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">اسم المتجر / النقطة</label>
          <input type="text" id="add-user-store" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">البريد الإلكتروني</label>
          <input type="email" id="add-user-email" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">رقم الهاتف</label>
          <input type="text" id="add-user-phone" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">كلمة المرور المؤقتة</label>
          <input type="password" id="add-user-password" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الدور والصلاحية</label>
            <select id="add-user-role" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="subscriber">مشترك عادي</option>
              <option value="pos">نقطة بيع (POS)</option>
              <option value="agent">وكيل معتمد (Agent)</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">فئة التسعير</label>
            <select id="add-user-tier" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="">الفئة الافتراضية</option>
            </select>
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-user')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ وإضافة</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 2. EDIT USER MODAL -->
  <div id="modal-edit-user" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">تعديل المشترك</h3>
        <button onclick="closeModal('modal-edit-user')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleUpdateUser(event)" class="space-y-3">
        <input type="hidden" id="edit-user-id">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الاسم الكامل</label>
          <input type="text" id="edit-user-name" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">اسم المتجر</label>
          <input type="text" id="edit-user-store" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">رقم الهاتف</label>
          <input type="text" id="edit-user-phone" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الدور والصلاحية</label>
            <select id="edit-user-role" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="subscriber">مشترك عادي</option>
              <option value="pos">نقطة بيع</option>
              <option value="agent">وكيل معتمد</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">فئة التسعير</label>
            <select id="edit-user-tier" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="">الفئة الافتراضية</option>
            </select>
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-edit-user')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ التعديلات</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 3. WALLET ADJUST MODAL -->
  <div id="modal-wallet-adjust" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">تعديل الرصيد</h3>
        <button onclick="closeModal('modal-wallet-adjust')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleWalletAdjustment(event)" class="space-y-3">
        <input type="hidden" id="wallet-adjust-user-id">
        <div class="p-3 bg-slate-50 rounded-2xl border border-slate-100">
          <span class="text-[10px] text-slate-400 block font-bold">المشترك المستهدف:</span>
          <span id="wallet-adjust-user-name" class="text-xs font-bold text-slate-800 block">...</span>
          <span id="wallet-adjust-current-balance" class="text-xs font-black text-emerald-600 block mt-0.5">الرصيد الحالي: 0 د.ع</span>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">نوع الحركة</label>
            <select id="wallet-adjust-type" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="credit">إيداع / شحن (+)</option>
              <option value="debit">خصم / سحب (-)</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">المبلغ (د.ع)</label>
            <input type="number" id="wallet-adjust-amount" required min="250" placeholder="مثال: 50000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-slate-800 outline-none focus:border-brand">
          </div>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">السبب / الملاحظة (Audit Memo)</label>
          <input type="text" id="wallet-adjust-notes" required placeholder="مثال: شحن رصيد نقدي مباشر" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-wallet-adjust')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl text-xs font-bold shadow-sm transition">تنفيذ القيد المحاسبي</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 4. ADD CARD MODAL -->
  <div id="modal-add-card" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">إضافة بطاقة</h3>
        <button onclick="closeModal('modal-add-card')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleSaveDigitalCard(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">اسم البطاقة (مثال: آسيا سيل 5000)</label>
          <input type="text" id="card-name" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">سعر الشراء (التكلفة د.ع)</label>
            <input type="number" id="card-cost" required min="100" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">سعر البيع الافتراضي (د.ع)</label>
            <input type="number" id="card-price" required min="100" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
          </div>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الفئة / الشركة (Category ID)</label>
          <input type="text" id="card-category" required placeholder="مثال: asiacell أو zain" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-card')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ البطاقة</button>
        </div>
      </form>
    </div>
  </div>
`
}

func GetModalsFintech() string {
	return `  <!-- 5. SET TIER PRICE MODAL -->
  <div id="modal-set-tier-price" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">تسعير فئة</h3>
        <button onclick="closeModal('modal-set-tier-price')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleSaveTierPrice(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الفئة المستهدفة</label>
          <select id="tier-price-tier-select" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand"></select>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">رمز المنتج (Product ID)</label>
          <input type="text" id="tier-price-product-id" required placeholder="مثال: asiacell_5000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">السعر المعتمد لهذه الفئة (د.ع)</label>
          <input type="number" id="tier-price-amount" required min="100" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-set-tier-price')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ السعر</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 6. SET SUBSCRIBER OVERRIDE MODAL -->
  <div id="modal-set-subscriber-override" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">سعر مخصص</h3>
        <button onclick="closeModal('modal-set-subscriber-override')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleSaveSubscriberOverride(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">المشترك المستهدف</label>
          <select id="sub-override-user-select" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand"></select>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">رمز المنتج (Product ID)</label>
          <input type="text" id="sub-override-product-id" required placeholder="مثال: asiacell_5000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">السعر الحصري (د.ع)</label>
          <input type="number" id="sub-override-price" required min="100" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-emerald-600 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-set-subscriber-override')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl text-xs font-bold shadow-sm transition">تطبيق السعر الحصري</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 7. SET AGENT COMMISSION MODAL -->
  <div id="modal-set-agent-commission" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">عمولة وكيل</h3>
        <button onclick="closeModal('modal-set-agent-commission')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleSaveAgentCommission(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الوكيل المستفيد</label>
          <select id="agent-comm-user-select" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand"></select>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">رمز المنتج (* لكافة المنتجات)</label>
          <input type="text" id="agent-comm-product-id" value="*" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">نوع العمولة</label>
            <select id="agent-comm-type" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="percentage">نسبة مئوية (%)</option>
              <option value="fixed_amount">مبلغ ثابت (د.ع)</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">قيمة العمولة</label>
            <input type="number" id="agent-comm-value" step="0.1" required min="0" placeholder="مثال: 2.5 أو 500" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-slate-800 outline-none focus:border-brand">
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-set-agent-commission')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ العمولة</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 8. SET CATEGORY THRESHOLD MODAL -->
  <div id="modal-set-category-threshold" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">حد النفاد</h3>
        <button onclick="closeModal('modal-set-category-threshold')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleSaveCategoryThreshold(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الفئة / الشركة (Category ID)</label>
          <input type="text" id="threshold-cat-id" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الحد الأدنى لعدد الكروت المتاحة</label>
          <input type="number" id="threshold-min-count" required min="1" value="5" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-rose-600 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-set-category-threshold')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-rose-600 hover:bg-rose-700 text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ الحد الأدنى</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 9. SET CREDIT LIMIT MODAL -->
  <div id="modal-set-credit-limit" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">تعديل الائتمان</h3>
        <button onclick="closeModal('modal-set-credit-limit')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="handleSaveCreditLimit(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الوكيل المستهدف</label>
          <select id="credit-limit-user-select" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand"></select>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">الائتمان المسموح به (د.ع)</label>
          <input type="number" id="credit-limit-amount" required min="0" placeholder="مثال: 5000000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-slate-800 outline-none focus:border-brand">
          <p class="text-[10px] text-slate-400 mt-1">يسمح للوكيل بالسحب على المكشوف حتى هذا السقف داخل المعاملات المحاسبية.</p>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-set-credit-limit')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ السقف</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 10. STATEMENT MODAL (PRINTABLE) -->
  <div id="modal-statement" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-3xl bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4 max-h-[90vh] flex flex-col">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <div>
          <h3 class="text-base font-black text-slate-800">كشف الحساب</h3>
          <p id="statement-header-info" class="text-xs text-slate-400">...</p>
        </div>
        <div class="flex items-center gap-2">
          <button onclick="shareStatementWhatsApp()" class="px-3 py-1.5 bg-emerald-50 text-emerald-700 hover:bg-emerald-100 rounded-xl text-xs font-bold flex items-center gap-1.5"><i class="fa-brands fa-whatsapp"></i><span>مشاركة واتساب</span></button>
          <button onclick="window.print()" class="px-3 py-1.5 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl text-xs font-bold flex items-center gap-1.5"><i class="fa-solid fa-print"></i><span>طباعة PDF</span></button>
          <button onclick="closeModal('modal-statement')" class="text-slate-400 hover:text-slate-600 p-1"><i class="fa-solid fa-xmark text-lg"></i></button>
        </div>
      </div>
      <div class="flex-1 overflow-y-auto space-y-4 text-xs font-medium" id="statement-content-body">
        <!-- populated dynamically -->
      </div>
    </div>
  </div>

  <!-- MODAL: ADD / EDIT GUARD MINIMUM MARGIN (Stage 1) -->
  <div id="modal-add-guard" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="bg-white rounded-3xl p-6 w-full max-w-md shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="font-black text-sm text-slate-800 flex items-center gap-2">
          <i class="fa-solid fa-shield-halved text-brand"></i>
          <span>ضبط الهامش الأدنى (حماية الأسعار)</span>
        </h3>
        <button onclick="closeModal('modal-add-guard')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form id="form-add-guard" onsubmit="submitAddGuardSetting(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">معرف الصنف أو المنتج</label>
          <input type="text" id="guard-item-id" required placeholder="اكتب * لتعيين هامش عام، أو اكتب رمز الصنف" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
          <span class="text-[10px] text-slate-400 block mt-1">استخدم * لتطبيقه كقاعدة افتراضية عامة لجميع المنتجات غير المخصصة</span>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الهامش الأدنى (د.ع)</label>
            <input type="number" id="guard-margin-iqd" min="0" value="0" step="250" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الهامش الأدنى (%)</label>
            <input type="number" id="guard-margin-pct" min="0" max="100" value="0" step="0.5" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
          </div>
        </div>
        <div class="flex items-center gap-2 pt-1">
          <input type="checkbox" id="guard-is-active" checked class="w-4 h-4 accent-brand cursor-pointer">
          <label for="guard-is-active" class="text-xs font-bold text-slate-700 cursor-pointer">تفعيل فحص الهامش</label>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-guard')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ قاعدة الهامش</button>
        </div>
      </form>
    </div>
  </div>
`
}

func GetModalsEnterprise() string {
	return `  <!-- 11. SUBSCRIBER 360 MODAL -->
  <div id="modal-subscriber-360" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-4xl bg-white rounded-3xl shadow-2xl border border-slate-100 max-h-[90vh] flex flex-col overflow-hidden">
      <div class="flex items-center justify-between p-6 border-b border-slate-100 bg-slate-50/50">
        <div class="flex items-center gap-3">
          <div class="w-12 h-12 rounded-2xl bg-blue-50 text-brand flex items-center justify-center text-xl font-black">
            <i class="fa-solid fa-crosshairs"></i>
          </div>
          <div>
            <div class="flex items-center gap-2">
              <h3 id="sub360-name" class="text-base font-black text-slate-800">ملف المشترك</h3>
              <span id="sub360-role-badge" class="px-2 py-0.5 rounded-full text-[10px] font-black bg-blue-50 text-brand">مشترك</span>
              <span id="sub360-status-badge" class="px-2 py-0.5 rounded-full text-[10px] font-black bg-emerald-50 text-emerald-600">نشط</span>
            </div>
            <p id="sub360-email-phone" class="text-xs text-slate-400 mt-0.5 font-mono">user@example.com</p>
          </div>
        </div>
        <button onclick="closeModal('modal-subscriber-360')" class="text-slate-400 hover:text-slate-600 p-2"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>

      <div class="p-6 overflow-y-auto space-y-6">
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-200">
            <span class="text-[10px] text-slate-400 block font-bold">الرصيد المالي الحالي</span>
            <span id="sub360-wallet" class="text-base font-black text-emerald-600">0 د.ع</span>
          </div>
          <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-200">
            <span class="text-[10px] text-slate-400 block font-bold">الائتمان</span>
            <span id="sub360-credit-limit" class="text-base font-black text-blue-600">0 د.ع</span>
          </div>
          <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-200">
            <span class="text-[10px] text-slate-400 block font-bold">حالة الاشتراك الإلكتروني</span>
            <span id="sub360-plan" class="text-xs font-black text-purple-600 block mt-1">بدون باقة</span>
          </div>
          <div class="p-3.5 bg-slate-50 rounded-2xl border border-slate-200">
            <span class="text-[10px] text-slate-400 block font-bold">الأيام المتبقية بالاشتراك</span>
            <span id="sub360-days-left" class="text-base font-black text-slate-800">0 يوم</span>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div class="p-4 bg-emerald-50/50 rounded-2xl border border-emerald-100">
            <div class="flex items-center justify-between text-xs font-bold text-emerald-800 mb-1">
              <span>مشتريات البطاقات</span>
              <i class="fa-solid fa-ticket text-emerald-600"></i>
            </div>
            <div id="sub360-vouchers" class="text-sm font-black text-emerald-700">0 كارت (0 د.ع)</div>
          </div>
          <div class="p-4 bg-amber-50/50 rounded-2xl border border-amber-100">
            <div class="flex items-center justify-between text-xs font-bold text-amber-800 mb-1">
              <span>الديون والآجل النشط</span>
              <i class="fa-solid fa-receipt text-amber-600"></i>
            </div>
            <div id="sub360-debts" class="text-sm font-black text-amber-700">0 د.ع متبقي</div>
          </div>
          <div class="p-4 bg-purple-50/50 rounded-2xl border border-purple-100">
            <div class="flex items-center justify-between text-xs font-bold text-purple-800 mb-1">
              <span>الأرباح المحققة للمتجر</span>
              <i class="fa-solid fa-chart-line text-purple-600"></i>
            </div>
            <div id="sub360-profits" class="text-sm font-black text-purple-700">0 د.ع</div>
          </div>
        </div>

        <div class="p-4 bg-slate-50 rounded-2xl border border-slate-200 space-y-2">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <i class="fa-solid fa-fingerprint text-brand"></i>
              <h4 class="text-xs font-black text-slate-800">بصمة الجهاز والتحكم بالأمان</h4>
            </div>
            <div id="sub360-device-lock-badge">
              <span class="px-2 py-0.5 rounded-full text-[10px] font-black bg-slate-100 text-slate-600">غير مقيد</span>
            </div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-2 text-[11px] font-mono text-slate-600 pt-1">
            <div><span class="text-slate-400 block font-sans">معرف الجهاز:</span> <span id="sub360-device-id" class="font-bold">غير مربوط</span></div>
            <div><span class="text-slate-400 block font-sans">موديل الجهاز:</span> <span id="sub360-device-model" class="font-bold">غير متوفر</span></div>
            <div><span class="text-slate-400 block font-sans">إصدار التطبيق / نبض:</span> <span id="sub360-app-version" class="font-bold">v1.0.0</span></div>
          </div>
        </div>

        <div class="space-y-2">
          <h4 class="text-xs font-black text-slate-800 flex items-center gap-1.5">
            <i class="fa-solid fa-book-journal-whills text-brand"></i>
            <span>آخر حركات السجل المالي</span>
          </h4>
          <div class="border border-slate-200 rounded-2xl overflow-hidden">
            <table class="w-full text-right border-collapse text-[11px]">
              <thead class="bg-slate-50 text-slate-400 font-bold border-b border-slate-200">
                <tr>
                  <th class="py-2 px-3">رقم العملية</th>
                  <th class="py-2 px-3">النوع</th>
                  <th class="py-2 px-3">مدين (سحب)</th>
                  <th class="py-2 px-3">دائن (إيداع)</th>
                  <th class="py-2 px-3">الرصيد بعدها</th>
                  <th class="py-2 px-3">التوقيت</th>
                </tr>
              </thead>
              <tbody id="sub360-ledger-table" class="divide-y divide-slate-100 font-medium">
                <tr><td colspan="6" class="py-4 text-center text-slate-400">لا توجد حركات سابقة</td></tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <div class="p-4 border-t border-slate-100 bg-slate-50/50 flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <button id="sub360-btn-impersonate" class="px-3.5 py-2 bg-teal-600 hover:bg-teal-700 text-white rounded-xl text-xs font-bold transition flex items-center gap-1.5 shadow-sm">
            <i class="fa-solid fa-user-secret"></i><span>معاينة الحساب</span>
          </button>
          <button id="sub360-btn-lock" class="px-3.5 py-2 bg-amber-600 hover:bg-amber-700 text-white rounded-xl text-xs font-bold transition flex items-center gap-1.5 shadow-sm">
            <i class="fa-solid fa-lock"></i><span>قفل الجهاز</span>
          </button>
          <button id="sub360-btn-kick" class="px-3.5 py-2 bg-rose-600 hover:bg-rose-700 text-white rounded-xl text-xs font-bold transition flex items-center gap-1.5 shadow-sm">
            <i class="fa-solid fa-arrow-right-from-bracket"></i><span>طرد الجلسات</span>
          </button>
        </div>
        <button onclick="closeModal('modal-subscriber-360')" class="px-4 py-2 bg-slate-200 hover:bg-slate-300 text-slate-700 rounded-xl text-xs font-bold">إغلاق</button>
      </div>
    </div>
  </div>

  <!-- 12. BULK ACTION MODAL -->
  <div id="modal-bulk-action" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-xl bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-xl bg-purple-50 text-purple-600 flex items-center justify-center text-sm font-black"><i class="fa-solid fa-layer-group"></i></div>
          <h3 class="text-sm font-black text-slate-800">معاينة</h3>
        </div>
        <button onclick="closeModal('modal-bulk-action')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>

      <div class="space-y-3">
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">تصفية حسب الدور</label>
            <select id="bulk-filter-role" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="">كافة الأدوار</option>
              <option value="subscriber">مشترك عادي (Subscriber)</option>
              <option value="pos">نقطة بيع (POS)</option>
              <option value="agent">وكيل معتمد (Agent)</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">حالة الحساب</label>
            <select id="bulk-filter-status" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="">كافة الحالات</option>
              <option value="active">نشط ومفعل</option>
              <option value="pending">بانتظار الموافقة</option>
              <option value="frozen">محفظة مجمدة</option>
            </select>
          </div>
        </div>

        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">نوع الإجراء الجماعي</label>
          <select id="bulk-action-type" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-purple-700 outline-none focus:border-brand">
            <option value="send_notification">إرسال إشعار جماعي لجميع الهواتف</option>
            <option value="force_sync">إجبار مزامنة فورية للبيانات والأجهزة</option>
            <option value="freeze_wallets">تجميد المحافظ احترازياً</option>
            <option value="unfreeze_wallets">فك تجميد المحافظ</option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">نص الرسالة / المعاملات</label>
          <input type="text" id="bulk-param-msg" placeholder="اكتب نص الإشعار هنا..." class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>

        <div id="bulk-preview-box" class="p-3 bg-purple-50 rounded-2xl border border-purple-100 text-xs hidden">
          <p id="bulk-preview-text" class="text-purple-700 font-bold">عدد الحسابات المستهدفة: 0 حساب</p>
        </div>

        <div class="flex items-center justify-between pt-2 border-t border-slate-100">
          <button type="button" onclick="runBulkPreview()" class="px-4 py-2 bg-purple-100 hover:bg-purple-200 text-purple-700 rounded-xl text-xs font-bold transition">معاينة مسبقة</button>
          <div class="flex items-center gap-2">
            <button type="button" onclick="closeModal('modal-bulk-action')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
            <button type="button" onclick="runBulkExecute()" class="px-5 py-2 bg-purple-600 hover:bg-purple-700 text-white rounded-xl text-xs font-bold shadow-sm transition">تنفيذ</button>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- 13. DEVICE CONTROLS MODAL -->
  <div id="modal-device-controls" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">الأجهزة والجلسات</h3>
        <button onclick="closeModal('modal-device-controls')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <input type="hidden" id="devctrl-user-id">
      <div class="space-y-3">
        <div class="p-3 bg-slate-50 rounded-2xl border border-slate-200 space-y-1">
          <span class="text-[10px] text-slate-400 font-bold block">معرف الجهاز المقترن حالياً (Fingerprint):</span>
          <span id="devctrl-device-id" class="text-xs font-mono font-bold text-slate-700 block truncate">لا يوجد</span>
          <span id="devctrl-lock-status" class="text-[11px] font-black text-slate-600 block mt-1">الحالة: غير مقفل</span>
        </div>
        <div class="space-y-2 pt-2">
          <button type="button" onclick="submitToggleDeviceLock()" id="devctrl-btn-toggle-lock" class="w-full py-2.5 px-3 bg-amber-50 hover:bg-amber-100 text-amber-700 border border-amber-200 rounded-xl text-xs font-bold transition flex items-center justify-between">
            <span class="flex items-center gap-2"><i class="fa-solid fa-lock"></i><span id="devctrl-toggle-text">قفل الجهاز</span></span>
          </button>
          <button type="button" onclick="submitResetDevice()" class="w-full py-2.5 px-3 bg-blue-50 hover:bg-blue-100 text-blue-700 border border-blue-200 rounded-xl text-xs font-bold transition flex items-center justify-between">
            <span class="flex items-center gap-2"><i class="fa-solid fa-arrows-rotate"></i><span>إعادة ضبط الجهاز</span></span>
          </button>
          <button type="button" onclick="submitKickSessions()" class="w-full py-2.5 px-3 bg-rose-50 hover:bg-rose-100 text-rose-700 border border-rose-200 rounded-xl text-xs font-bold transition flex items-center justify-between">
            <span class="flex items-center gap-2"><i class="fa-solid fa-power-off"></i><span>طرد الجلسات</span></span>
          </button>
        </div>
      </div>
      <div class="flex items-center justify-end pt-2 border-t border-slate-100">
        <button type="button" onclick="closeModal('modal-device-controls')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إغلاق</button>
      </div>
    </div>
  </div>

  <!-- 14. ADD COUPON MODAL -->
  <div id="modal-add-coupon" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">إضافة كود خصم</h3>
        <button onclick="closeModal('modal-add-coupon')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="submitAddCoupon(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">كود الخصم (Coupon Code)</label>
          <input type="text" id="coupon-code" required placeholder="مثال: VENUS2026" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black uppercase text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">نوع الخصم</label>
            <select id="coupon-type" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="percentage">نسبة مئوية (%)</option>
              <option value="fixed_amount">مبلغ ثابت (د.ع)</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">قيمة الخصم</label>
            <input type="number" id="coupon-value" required min="1" placeholder="10 أو 5000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-black text-purple-700 outline-none focus:border-brand">
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الحد الأقصى الإجمالي</label>
            <input type="number" id="coupon-limit-total" min="1" value="100" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-700 outline-none focus:border-brand">
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الحد لكل مستخدم</label>
            <input type="number" id="coupon-limit-user" min="1" value="1" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-700 outline-none focus:border-brand">
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-coupon')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-purple-600 hover:bg-purple-700 text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ الكود</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 15. ADD IP RULE MODAL -->
  <div id="modal-add-ip-rule" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">إضافة قاعدة IP</h3>
        <button onclick="closeModal('modal-add-ip-rule')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form onsubmit="submitAddIPRule(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">عنوان IP أو النطاق CIDR</label>
          <input type="text" id="iprule-address" required placeholder="مثال: 185.190.140.2" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">نوع القاعدة</label>
            <select id="iprule-type" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="blacklist">حظر تام (Blacklist)</option>
              <option value="whitelist">قائمة بيضاء (Whitelist)</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">المدة (ساعات)</label>
            <input type="number" id="iprule-hours" min="0" value="24" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-700 outline-none focus:border-brand">
          </div>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">السبب / الملاحظة</label>
          <input type="text" id="iprule-reason" required placeholder="محاولات تخمين متكررة" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-ip-rule')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-rose-600 hover:bg-rose-700 text-white rounded-xl text-xs font-bold shadow-sm transition">تطبيق القاعدة</button>
        </div>
      </form>
    </div>
  </div>

  <!-- 16. ADD API KEY MODAL -->
  <div id="modal-add-api-key" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="w-full max-w-md bg-white rounded-3xl shadow-2xl border border-slate-100 p-6 space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="text-sm font-black text-slate-800">توليد مفتاح API</h3>
        <button onclick="closeModal('modal-add-api-key')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form id="form-add-apikey" onsubmit="submitAddAPIKey(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">اسم / وصف المفتاح</label>
          <input type="text" id="apikey-label" required placeholder="تكامل نظام المحاسبة الخارجي" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الدور والصلاحية</label>
            <select id="apikey-role" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="external_api">خدمات خارجية (External API)</option>
              <option value="pos">نقطة بيع مخصصة</option>
              <option value="admin">إدارة كاملة</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">معدل الطلبات / دقيقة</label>
            <input type="number" id="apikey-rate-limit" min="10" value="60" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-700 outline-none focus:border-brand">
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-api-key')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">توليد المفتاح</button>
        </div>
      </form>
      <div id="apikey-result-box" class="p-4 bg-emerald-50 rounded-2xl border border-emerald-200 space-y-2 hidden">
        <span class="text-xs font-black text-emerald-800 block">انسخ المفتاح الآن! لن يتم عرضه مرة أخرى:</span>
        <div class="p-2.5 bg-white rounded-xl border border-emerald-200 font-mono text-xs font-bold text-slate-800 select-all break-all" id="apikey-secret-value">vsk_live_...</div>
        <button type="button" onclick="copyAPIKeyToClipboard()" class="w-full py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl text-xs font-bold shadow-sm transition flex items-center justify-center gap-2">
          <i class="fa-solid fa-copy"></i><span>نسخ المفتاح للحافظة</span>
        </button>
      </div>
    </div>
  </div>

  <!-- MODAL: ADD NOTIFY CHANNEL (Stage 3) -->
  <div id="modal-add-notify-channel" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="bg-white rounded-3xl p-6 w-full max-w-md shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="font-black text-sm text-slate-800 flex items-center gap-2">
          <i class="fa-solid fa-bullhorn text-brand"></i>
          <span>إضافة قناة إشعارات جديدة</span>
        </h3>
        <button onclick="closeModal('modal-add-notify-channel')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form id="form-add-notify-channel" onsubmit="submitAddNotifyChannel(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">نوع القناة</label>
          <select id="notify-channel-type" onchange="toggleNotifyFields()" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
            <option value="telegram">Telegram Bot</option>
            <option value="whatsapp">WhatsApp Gateway</option>
            <option value="sms">SMS Gateway</option>
            <option value="push">Push Notification</option>
          </select>
        </div>
        <div id="notify-field-token">
          <label class="block text-xs font-bold text-slate-700 mb-1">رمز البوت (Bot Token / API Key)</label>
          <input type="text" id="notify-token" placeholder="bot123456:ABC-DEF..." class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div id="notify-field-target">
          <label class="block text-xs font-bold text-slate-700 mb-1">المعرف أو الرابط (Chat ID / Endpoint URL)</label>
          <input type="text" id="notify-target" placeholder="-100123456789 أو https://api.gateway.com/send" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الأولوية (Priority)</label>
            <input type="number" id="notify-priority" min="1" max="10" value="1" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
            <span class="text-[10px] text-slate-400 block mt-0.5">1 يُنفَّذ أولاً ثم 2 كبديل</span>
          </div>
          <div class="flex items-center gap-2 pt-6">
            <input type="checkbox" id="notify-is-active" checked class="w-4 h-4 accent-brand cursor-pointer">
            <label for="notify-is-active" class="text-xs font-bold text-slate-700 cursor-pointer">القناة نشطة</label>
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-notify-channel')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ القناة</button>
        </div>
      </form>
    </div>
  </div>

  <!-- MODAL: ADD REWARD RULE (Stage 4) -->
  <div id="modal-add-reward-rule" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="bg-white rounded-3xl p-6 w-full max-w-md shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="font-black text-sm text-slate-800 flex items-center gap-2">
          <i class="fa-solid fa-gift text-purple-600"></i>
          <span>إضافة قاعدة مكافأة جديدة</span>
        </h3>
        <button onclick="closeModal('modal-add-reward-rule')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form id="form-add-reward-rule" onsubmit="submitAddRewardRule(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">اسم القاعدة / الحملة</label>
          <input type="text" id="reward-rule-name" required placeholder="مثال: حافز بطل المبيعات الشهري" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">المعيار (Metric)</label>
            <select id="reward-rule-metric" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="sales_amount">إجمالي المبيعات (د.ع)</option>
              <option value="sales_count">عدد عمليات البيع</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-bold text-slate-700 mb-1">الفترة الزمنية</label>
            <select id="reward-rule-period" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
              <option value="monthly">شهري</option>
              <option value="weekly">أسبوعي</option>
            </select>
          </div>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">المستهدف المطلوب (Target)</label>
          <input type="number" id="reward-rule-target" required min="1" step="1000" placeholder="مثال: 5000000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">مبلغ المكافأة عند الإنجاز (د.ع)</label>
          <input type="number" id="reward-rule-reward" required min="1" step="1000" placeholder="مثال: 50000" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="flex items-center gap-2 pt-2">
          <input type="checkbox" id="reward-rule-is-active" checked class="w-4 h-4 accent-brand cursor-pointer">
          <label for="reward-rule-is-active" class="text-xs font-bold text-slate-700 cursor-pointer">القاعدة مفعلة</label>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-reward-rule')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ وتفعيل</button>
        </div>
      </form>
    </div>
  </div>

  <!-- MODAL: OVERRIDE RISK (Stage 4) -->
  <div id="modal-override-risk" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="bg-white rounded-3xl p-6 w-full max-w-md shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="font-black text-sm text-slate-800 flex items-center gap-2">
          <i class="fa-solid fa-user-shield text-brand"></i>
          <span>تعديل تقييم المخاطر وفك التجميد</span>
        </h3>
        <button onclick="closeModal('modal-override-risk')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form id="form-override-risk" onsubmit="submitOverrideRisk(event)" class="space-y-3">
        <input type="hidden" id="override-risk-merchant-id">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">المشترك / التاجر</label>
          <div id="override-risk-merchant-info" class="p-2.5 bg-slate-50 rounded-xl font-bold text-xs text-slate-800 border border-slate-200">#</div>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">درجة المخاطرة اليدوية الجديدة (0-100)</label>
          <input type="number" id="override-risk-score" min="0" max="100" required class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
          <span class="text-[10px] text-slate-400 block mt-0.5">0-40 آمن، 41-79 تحذير، 80-100 تجميد فوري</span>
        </div>
        <div class="flex items-center gap-2 pt-2">
          <input type="checkbox" id="override-risk-unfreeze" checked class="w-4 h-4 accent-brand cursor-pointer">
          <label for="override-risk-unfreeze" class="text-xs font-bold text-slate-700 cursor-pointer">إلغاء التجميد فورياً والسماح بالعمليات</label>
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-override-risk')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">حفظ التعديل</button>
        </div>
      </form>
    </div>
  </div>

  <!-- MODAL: ADD API KEY V2 (Stage 4) -->
  <div id="modal-add-api-key-v2" class="fixed inset-0 z-50 flex items-center justify-center p-4 modal-backdrop hidden">
    <div class="bg-white rounded-3xl p-6 w-full max-w-md shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-slate-100 pb-3">
        <h3 class="font-black text-sm text-slate-800 flex items-center gap-2">
          <i class="fa-solid fa-network-wired text-brand"></i>
          <span>توليد مفتاح برمجي B2B (API Key v2)</span>
        </h3>
        <button onclick="closeModal('modal-add-api-key-v2')" class="text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
      </div>
      <form id="form-add-api-key-v2" onsubmit="submitAddApiKeyV2(event)" class="space-y-3">
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">اسم المفتاح / الشريك</label>
          <input type="text" id="apikey-v2-label" required placeholder="مثال: تطبيق نقاط بيع بغداد كاش" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">صلاحيات الوصول (Scopes)</label>
          <div class="grid grid-cols-2 gap-2 text-xs font-bold text-slate-700 p-2.5 bg-slate-50 rounded-xl border border-slate-200">
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" name="apikey-scope" value="read" checked class="accent-brand"><span>قراءة (read)</span>
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" name="apikey-scope" value="sales" checked class="accent-brand"><span>المبيعات (sales)</span>
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" name="apikey-scope" value="vouchers" class="accent-brand"><span>البطاقات (vouchers)</span>
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" name="apikey-scope" value="sync" class="accent-brand"><span>المزامنة (sync)</span>
            </label>
          </div>
        </div>
        <div>
          <label class="block text-xs font-bold text-slate-700 mb-1">سقف الطلبات في الدقيقة (Rate Limit)</label>
          <input type="number" id="apikey-v2-rate-limit" min="10" max="600" value="60" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-bold text-slate-800 outline-none focus:border-brand">
        </div>
        <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100">
          <button type="button" onclick="closeModal('modal-add-api-key-v2')" class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl text-xs font-bold">إلغاء</button>
          <button type="submit" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl text-xs font-bold shadow-sm transition">توليد المفتاح المشفر</button>
        </div>
      </form>
      <div id="apikey-v2-result-box" class="p-4 bg-emerald-50 rounded-2xl border border-emerald-200 space-y-2 hidden">
        <span class="text-xs font-black text-emerald-800 block">احفظ هذا المفتاح فوراً! لن يظهر لك مرة ثانية:</span>
        <div class="p-2.5 bg-white rounded-xl border border-emerald-200 font-mono text-xs font-bold text-slate-800 select-all break-all" id="apikey-v2-secret-value">upos_...</div>
        <button type="button" onclick="copyApiKeyV2ToClipboard()" class="w-full py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl text-xs font-bold shadow-sm transition flex items-center justify-center gap-2">
          <i class="fa-solid fa-copy"></i><span>نسخ المفتاح للحافظة</span>
        </button>
      </div>
    </div>
  </div>
`
}

func GetToast() string {
	return `  <!-- GLOBAL TOAST NOTIFICATION -->
  <div id="toast" class="fixed bottom-5 left-5 z-50 transform translate-y-20 opacity-0 transition-all duration-300 pointer-events-none">
    <div id="toast-inner" class="flex items-center gap-2.5 px-4 py-3 rounded-2xl shadow-xl text-xs font-bold text-white bg-slate-900 border border-slate-800">
      <i id="toast-icon" class="fa-solid fa-circle-check text-emerald-400"></i>
      <span id="toast-text">تمت العملية بنجاح</span>
    </div>
  </div>
`
}
