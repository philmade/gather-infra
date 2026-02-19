/**
 * Shop Pages — 3 page renderers using hash router
 * Routes: /, /order, /status/:id
 */

const ShopPages = {
    /**
     * Menu page — category cards + CTAs
     */
    async menu() {
        render('#app', Templates.spinner());

        try {
            const data = await api.getMenu();
            const categories = data.categories || [];

            const categoryCards = categories.map(cat => `
                <div class="category-card card card-hover" onclick="navigate('/browse/${encodeURIComponent(cat.id)}')">
                    <div class="category-name">${escapeHtml(cat.name)}</div>
                    <div class="category-count">${cat.count} items</div>
                    <span class="btn btn-ghost btn-sm" style="margin-top: 0.5rem;">Browse</span>
                </div>
            `).join('');

            render('#app', `
                <div class="shop-page">
                    <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 1rem; color: var(--text-primary);">
                        Gather Shop
                    </h1>
                    <p style="color: var(--text-secondary); margin-bottom: 1.5rem;">
                        Custom merch with your design &mdash; upload an image, pick a product, pay in BCH, and get it printed &amp; shipped.
                    </p>

                    <div class="shop-ctas">
                        <a href="#/browse/products" class="btn btn-primary">Order Merch</a>
                    </div>

                    <h2 class="section-title" style="margin-top: 2rem;">Categories</h2>
                    <div class="category-grid">${categoryCards}</div>
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Failed to load menu', err.message));
        }
    },

    /**
     * Browse category items
     */
    async browse({ params = {} } = {}) {
        render('#app', Templates.spinner());

        try {
            const data = await api.getMenuCategory(params.category);
            const items = data.items || [];

            const isProducts = params.category === 'products';
            const itemCards = items.map(item => `
                <div class="category-card card card-hover"
                     ${isProducts ? `onclick="navigate('/order?type=product&id=${encodeURIComponent(item.id)}')"` : ''}
                     style="${!item.available ? 'opacity: 0.5;' : ''}">
                    <div class="category-name">${escapeHtml(item.name)}</div>
                    <div style="color: var(--text-muted); font-size: 0.85rem;">
                        ${item.base_price_bch ? item.base_price_bch + ' BCH' : ''}
                        ${!item.available ? '<span class="badge badge-red" style="margin-left: 0.5rem;">Unavailable</span>' : ''}
                    </div>
                    ${isProducts ? '<span class="btn btn-ghost btn-sm" style="margin-top: 0.5rem;">Order</span>' : ''}
                </div>
            `).join('');

            render('#app', `
                <div class="shop-page">
                    ${Templates.breadcrumb([
                        { label: 'Shop', href: '#/' },
                        { label: params.category },
                    ])}

                    <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 1rem; color: var(--text-primary);">
                        ${escapeHtml(params.category)}
                    </h1>

                    <div class="category-grid">${itemCards}</div>

                    ${data.total_pages > 1 ? `
                        <div style="display: flex; gap: 1rem; justify-content: center; margin-top: 1.5rem;">
                            ${data.page > 1 ? `<button class="btn btn-ghost btn-sm" onclick="ShopPages._browsePage('${params.category}', ${data.page - 1})">Previous</button>` : ''}
                            <span style="color: var(--text-muted);">Page ${data.page} of ${data.total_pages}</span>
                            ${data.next ? `<button class="btn btn-ghost btn-sm" onclick="ShopPages._browsePage('${params.category}', ${data.page + 1})">Next</button>` : ''}
                        </div>
                    ` : ''}
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Failed to load category', err.message));
        }
    },

    /**
     * Order page — dispatches to ProductSelector
     */
    async order({ query = {} } = {}) {
        if (query.type === 'product' && query.id) {
            ProductSelector.render(query.id);
        } else {
            render('#app', `
                <div class="shop-page">
                    ${Templates.breadcrumb([
                        { label: 'Shop', href: '#/' },
                        { label: 'Order' },
                    ])}
                    ${Templates.emptyState('What would you like to order?', 'Browse our products and pick one to customize')}
                    <div class="shop-ctas" style="margin-top: 1rem;">
                        <a href="#/browse/products" class="btn btn-primary">Browse Products</a>
                    </div>
                </div>
            `);
        }
    },

    /**
     * Order status page
     */
    async status({ params = {} } = {}) {
        render('#app', Templates.spinner());

        try {
            const order = await api.getOrder(params.id);

            const statusColors = {
                awaiting_payment: 'badge-yellow',
                confirmed: 'badge-green',
                fulfilling: 'badge-purple',
                shipped: 'badge-green',
                complete: 'badge-green',
                failed: 'badge-red',
            };
            const statusClass = statusColors[order.status] || '';

            // Order summary
            const opts = order.product_options || {};
            const optList = Object.entries(opts).map(([k, v]) => `${escapeHtml(k)}: ${escapeHtml(v)}`).join(', ');
            const orderDetails = `
                <div class="card" style="padding: 1rem; margin-bottom: 1rem;">
                    <h3 style="font-weight: 600; margin-bottom: 0.5rem;">Product Order</h3>
                    <div style="color: var(--text-secondary);">
                        <div>Product: ${escapeHtml(order.product_id || '-')}</div>
                        ${optList ? `<div>Options: ${optList}</div>` : ''}
                        ${order.design_url ? `<div>Design: <a href="${escapeHtml(order.design_url)}" target="_blank" style="color: var(--accent);">View</a></div>` : ''}
                        ${order.gelato_order_id ? `<div>Gelato Order: ${escapeHtml(order.gelato_order_id)}</div>` : ''}
                        ${order.tracking_url ? `<div>Tracking: <a href="${escapeHtml(order.tracking_url)}" target="_blank" style="color: var(--accent);">${escapeHtml(order.tracking_url)}</a></div>` : ''}
                    </div>
                </div>`;

            // Payment section
            let paymentSection = '';
            if (order.status === 'awaiting_payment') {
                paymentSection = `
                    <div class="card" style="padding: 1rem; margin-bottom: 1rem;">
                        <h3 style="font-weight: 600; margin-bottom: 0.5rem;">Payment</h3>
                        <div class="payment-address">
                            <div style="font-size: 0.85rem; color: var(--text-muted); margin-bottom: 0.25rem;">Send BCH to:</div>
                            <code style="font-family: var(--font-mono); word-break: break-all;">${escapeHtml(order.payment_address)}</code>
                        </div>
                        <form onsubmit="event.preventDefault(); ShopPages._submitPayment('${escapeHtml(order.order_id)}');" style="margin-top: 1rem;">
                            <label class="form-label" for="tx-id">Transaction ID (64-char hex)</label>
                            <input type="text" class="form-input" id="tx-id" placeholder="BCH transaction hash..." minlength="64" maxlength="64" required>
                            <button type="submit" class="btn btn-primary btn-sm" id="pay-btn" style="margin-top: 0.5rem;">Submit Payment</button>
                        </form>
                    </div>`;
            } else if (order.paid && order.tx_id) {
                paymentSection = `
                    <div class="card" style="padding: 1rem; margin-bottom: 1rem;">
                        <h3 style="font-weight: 600; margin-bottom: 0.5rem;">Payment Confirmed</h3>
                        <div style="color: var(--text-secondary); font-size: 0.85rem;">
                            TX: <code style="font-family: var(--font-mono); word-break: break-all;">${escapeHtml(order.tx_id)}</code>
                        </div>
                    </div>`;
            }

            // Feedback section (show after payment)
            let feedbackSection = '';
            if (order.paid) {
                feedbackSection = `
                    <div class="card" style="padding: 1rem; margin-top: 1rem;">
                        <h3 style="font-weight: 600; margin-bottom: 0.5rem;">Feedback</h3>
                        <form onsubmit="event.preventDefault(); ShopPages._submitFeedback();">
                            <label class="form-label" for="fb-rating">Rating (1-5)</label>
                            <select class="form-select" id="fb-rating" required>
                                <option value="">Select...</option>
                                <option value="5">5 — Excellent</option>
                                <option value="4">4 — Good</option>
                                <option value="3">3 — OK</option>
                                <option value="2">2 — Poor</option>
                                <option value="1">1 — Terrible</option>
                            </select>
                            <label class="form-label" for="fb-message" style="margin-top: 0.5rem;">Message (optional)</label>
                            <textarea class="form-input" id="fb-message" rows="3" placeholder="How was your experience?"></textarea>
                            <button type="submit" class="btn btn-secondary btn-sm" id="fb-btn" style="margin-top: 0.5rem;">Send Feedback</button>
                        </form>
                        <div id="fb-result"></div>
                    </div>`;
            }

            render('#app', `
                <div class="shop-page">
                    ${Templates.breadcrumb([
                        { label: 'Shop', href: '#/' },
                        { label: 'Order Status' },
                    ])}

                    <div style="display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem;">
                        <h1 style="font-size: 1.5rem; font-weight: 700; color: var(--text-primary);">
                            Order ${escapeHtml(order.order_id)}
                        </h1>
                        <span class="badge ${statusClass}">${escapeHtml(order.status)}</span>
                    </div>

                    <div style="color: var(--accent); font-weight: 600; font-size: 1.1rem; margin-bottom: 1rem;">
                        ${escapeHtml(order.total_bch)} BCH
                    </div>

                    ${orderDetails}
                    ${paymentSection}
                    ${feedbackSection}
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Order not found', err.message));
        }
    },

    /**
     * Submit payment for an order
     */
    async _submitPayment(orderId) {
        const txId = document.getElementById('tx-id')?.value?.trim();
        if (!txId || txId.length !== 64) {
            alert('Transaction ID must be exactly 64 characters');
            return;
        }

        const btn = document.getElementById('pay-btn');
        await withLoadingButton(btn, 'Verifying...', async () => {
            await api.submitPayment(orderId, txId);
            // Reload the status page
            navigate(`/status/${orderId}`);
        });
    },

    /**
     * Submit feedback
     */
    async _submitFeedback() {
        const rating = parseInt(document.getElementById('fb-rating')?.value, 10);
        const message = document.getElementById('fb-message')?.value?.trim() || '';

        if (!rating || rating < 1 || rating > 5) {
            alert('Please select a rating');
            return;
        }

        const btn = document.getElementById('fb-btn');
        await withLoadingButton(btn, 'Sending...', async () => {
            await api.submitFeedback({ rating, message });
            render('#fb-result', '<div class="badge badge-green" style="margin-top: 0.5rem;">Thanks for your feedback!</div>');
        });
    },

    /**
     * Browse page navigation helper
     */
    _browsePage(category, page) {
        // Re-render with pagination — simple approach: reload browse
        navigate(`/browse/${category}?page=${page}`);
    },
};

window.ShopPages = ShopPages;

// Initialize router
new Router({
    '/': () => ShopPages.menu(),
    '/browse/:category': ({ params, query }) => ShopPages.browse({ params, query }),
    '/order': ({ query }) => ShopPages.order({ query }),
    '/status/:id': ({ params }) => ShopPages.status({ params }),
});
