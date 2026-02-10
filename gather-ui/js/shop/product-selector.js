/**
 * Product Selector — Design upload + product options + shipping form
 * Loads options from API, builds form, submits product order
 */

const ProductSelector = {
    _productId: null,
    _options: null,
    _designUrl: null,

    async render(productId) {
        this._productId = productId;
        this._designUrl = null;
        render('#app', Templates.spinner());

        try {
            const data = await api.getProductOptions(productId);
            this._options = data.options || {};

            const optionFields = Object.entries(this._options).map(([key, values]) => `
                <div>
                    <label class="form-label" for="opt-${escapeHtml(key)}">${escapeHtml(key)}</label>
                    <select class="form-select" id="opt-${escapeHtml(key)}" required>
                        <option value="">Select ${escapeHtml(key)}...</option>
                        ${values.map(v => `<option value="${escapeHtml(v)}">${escapeHtml(v)}</option>`).join('')}
                    </select>
                </div>
            `).join('');

            render('#app', `
                <div class="shop-page">
                    ${Templates.breadcrumb([
                        { label: 'Shop', href: '#/' },
                        { label: 'Products', href: '#/browse/products' },
                        { label: data.product_name || productId },
                    ])}

                    <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem; color: var(--text-primary);">
                        ${escapeHtml(data.product_name || productId)}
                    </h1>

                    <form class="product-form" onsubmit="event.preventDefault(); ProductSelector.submit();">
                        <h2 class="section-title">Your Design</h2>
                        <p style="color: var(--text-secondary); font-size: 0.85rem; margin-bottom: 0.75rem;">
                            Upload an image to print on your product. Accepted: PNG, JPG, WebP, SVG (max 20MB).
                        </p>
                        <div class="design-upload-area">
                            <input type="file" id="design-file" accept=".png,.jpg,.jpeg,.webp,.svg"
                                   onchange="ProductSelector.handleDesignUpload()" style="margin-bottom: 0.5rem;">
                            <div id="design-status"></div>
                            <div id="design-preview" style="margin-top: 0.5rem;"></div>
                        </div>

                        <h2 class="section-title" style="margin-top: 1.5rem;">Options</h2>
                        ${optionFields}

                        <h2 class="section-title" style="margin-top: 1.5rem;">Shipping Address</h2>
                        <div class="shipping-form">
                            <div class="shipping-row">
                                <div>
                                    <label class="form-label" for="ship-first">First Name</label>
                                    <input type="text" class="form-input" id="ship-first" required>
                                </div>
                                <div>
                                    <label class="form-label" for="ship-last">Last Name</label>
                                    <input type="text" class="form-input" id="ship-last" required>
                                </div>
                            </div>
                            <div>
                                <label class="form-label" for="ship-addr1">Address Line 1</label>
                                <input type="text" class="form-input" id="ship-addr1" required>
                            </div>
                            <div>
                                <label class="form-label" for="ship-addr2">Address Line 2</label>
                                <input type="text" class="form-input" id="ship-addr2">
                            </div>
                            <div class="shipping-row">
                                <div>
                                    <label class="form-label" for="ship-city">City</label>
                                    <input type="text" class="form-input" id="ship-city" required>
                                </div>
                                <div>
                                    <label class="form-label" for="ship-state">State/Province</label>
                                    <input type="text" class="form-input" id="ship-state">
                                </div>
                            </div>
                            <div class="shipping-row">
                                <div>
                                    <label class="form-label" for="ship-zip">Postal Code</label>
                                    <input type="text" class="form-input" id="ship-zip" required>
                                </div>
                                <div>
                                    <label class="form-label" for="ship-country">Country (2-letter code)</label>
                                    <input type="text" class="form-input" id="ship-country" maxlength="2" minlength="2" placeholder="US" required>
                                </div>
                            </div>
                            <div>
                                <label class="form-label" for="ship-email">Email</label>
                                <input type="email" class="form-input" id="ship-email" required>
                            </div>
                            <div>
                                <label class="form-label" for="ship-phone">Phone (optional)</label>
                                <input type="tel" class="form-input" id="ship-phone">
                            </div>
                        </div>

                        <button type="submit" class="btn btn-primary" id="product-submit-btn" style="margin-top: 1.5rem;">
                            Place Order
                        </button>
                    </form>
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Failed to load product options', err.message));
        }
    },

    async handleDesignUpload() {
        const input = document.getElementById('design-file');
        const statusEl = document.getElementById('design-status');
        const previewEl = document.getElementById('design-preview');
        const file = input?.files?.[0];

        if (!file) return;

        statusEl.innerHTML = '<span class="badge badge-yellow">Uploading...</span>';
        previewEl.innerHTML = '';

        try {
            const result = await api.uploadDesign(file);
            this._designUrl = result.design_url;
            statusEl.innerHTML = '<span class="badge badge-green">Design uploaded</span>';

            // Show preview for image types
            if (file.type.startsWith('image/') && file.type !== 'image/svg+xml') {
                const reader = new FileReader();
                reader.onload = (e) => {
                    previewEl.innerHTML = `<img src="${e.target.result}" alt="Design preview"
                        style="max-width: 200px; max-height: 200px; border-radius: var(--radius-sm); border: 1px solid var(--border);">`;
                };
                reader.readAsDataURL(file);
            }
        } catch (err) {
            statusEl.innerHTML = `<span class="badge badge-red">Upload failed: ${escapeHtml(err.message)}</span>`;
            this._designUrl = null;
        }
    },

    async submit() {
        // Gather selected options
        const options = {};
        for (const key of Object.keys(this._options || {})) {
            const val = document.getElementById(`opt-${key}`)?.value;
            if (!val) { alert(`Please select ${key}`); return; }
            options[key] = val;
        }

        // Gather shipping address
        const shipping_address = {
            first_name: document.getElementById('ship-first')?.value?.trim(),
            last_name: document.getElementById('ship-last')?.value?.trim(),
            address_line_1: document.getElementById('ship-addr1')?.value?.trim(),
            address_line_2: document.getElementById('ship-addr2')?.value?.trim() || '',
            city: document.getElementById('ship-city')?.value?.trim(),
            state: document.getElementById('ship-state')?.value?.trim() || '',
            post_code: document.getElementById('ship-zip')?.value?.trim(),
            country: document.getElementById('ship-country')?.value?.trim().toUpperCase(),
            email: document.getElementById('ship-email')?.value?.trim(),
            phone: document.getElementById('ship-phone')?.value?.trim() || '',
        };

        // Basic validation
        if (!shipping_address.first_name || !shipping_address.last_name) {
            alert('Please enter your name'); return;
        }
        if (!shipping_address.address_line_1 || !shipping_address.city || !shipping_address.post_code) {
            alert('Please enter your full address'); return;
        }
        if (shipping_address.country.length !== 2) {
            alert('Country must be a 2-letter code (e.g. US, GB)'); return;
        }
        if (!shipping_address.email) {
            alert('Email is required for shipping updates'); return;
        }

        const payload = {
            product_id: this._productId,
            options,
            shipping_address,
        };
        if (this._designUrl) {
            payload.design_url = this._designUrl;
        }

        const btn = document.getElementById('product-submit-btn');
        await withLoadingButton(btn, 'Placing order...', async () => {
            const result = await api.createProductOrder(payload);
            navigate(`/status/${result.order_id}`);
        });
    },
};

window.ProductSelector = ProductSelector;
