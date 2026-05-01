let chart = null;

async function fetchWeights(days) {
    const url = days > 0 ? `/api/weights?days=${days}` : '/api/weights';
    const resp = await fetch(url);
    if (!resp.ok) throw new Error('Failed to fetch weights');
    return resp.json();
}

function formatDate(dateStr) {
    const d = new Date(dateStr);
    return d.toLocaleDateString('en-AU', {
        day: '2-digit',
        month: 'short',
        year: 'numeric'
    });
}

function formatDateTime(dateStr) {
    const d = new Date(dateStr);
    return d.toLocaleDateString('en-AU', {
        day: '2-digit',
        month: 'short',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
}

function renderChart(weights) {
    const ctx = document.getElementById('weightChart').getContext('2d');

    // Chart data is chronological (oldest first) as {x: Date, y: kg}
    const sorted = [...weights].reverse();
    const data = sorted.map(w => ({ x: new Date(w.created_at), y: w.weight_kg }));

    if (chart) {
        chart.destroy();
    }

    chart = new Chart(ctx, {
        type: 'line',
        data: {
            datasets: [{
                label: 'Weight (kg)',
                data: data,
                borderColor: '#3b82f6',
                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                fill: true,
                tension: 0.3,
                pointRadius: 3,
                pointHoverRadius: 6,
                pointBackgroundColor: '#3b82f6'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false },
                tooltip: {
                    backgroundColor: '#1e293b',
                    titleColor: '#e2e8f0',
                    bodyColor: '#e2e8f0',
                    borderColor: '#334155',
                    borderWidth: 1,
                    callbacks: {
                        title: function(items) {
                            return formatDateTime(items[0].raw.x.toISOString());
                        },
                        label: function(ctx) {
                            return ctx.parsed.y.toFixed(2) + ' kg';
                        }
                    }
                }
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        tooltipFormat: 'dd MMM yyyy',
                        displayFormats: {
                            day: 'dd MMM',
                            week: 'dd MMM',
                            month: 'MMM yyyy',
                            year: 'yyyy'
                        }
                    },
                    ticks: { color: '#64748b', maxTicksLimit: 10 },
                    grid: { color: '#1e293b' }
                },
                y: {
                    ticks: {
                        color: '#64748b',
                        callback: function(v) { return v + ' kg'; }
                    },
                    grid: { color: '#334155' }
                }
            }
        }
    });
}

function renderSummary(weights) {
    const el = document.getElementById('summary');

    if (weights.length < 2) {
        el.innerHTML = '';
        return;
    }

    // weights are DESC by created_at — first is newest, last is oldest
    const newest = weights[0];
    const oldest = weights[weights.length - 1];
    const changeKg = newest.weight_kg - oldest.weight_kg;
    const changePct = (changeKg / oldest.weight_kg) * 100;
    const spanDays = (new Date(newest.created_at) - new Date(oldest.created_at)) / (1000 * 60 * 60 * 24);
    const rate = spanDays > 0 ? (changeKg / spanDays) * 7 : 0;

    const sign = changeKg > 0 ? '+' : '';
    const dir = changeKg > 0 ? 'up' : changeKg < 0 ? 'down' : '';

    el.innerHTML = `
        <div class="summary-card">
            <div class="label">Total change</div>
            <div class="value ${dir}">${sign}${changeKg.toFixed(2)} kg</div>
        </div>
        <div class="summary-card">
            <div class="label">Percent change</div>
            <div class="value ${dir}">${sign}${changePct.toFixed(2)}%</div>
        </div>
        <div class="summary-card">
            <div class="label">Average rate</div>
            <div class="value ${dir}">${sign}${rate.toFixed(2)} kg/week</div>
        </div>
    `;
}

function renderTable(weights) {
    const tbody = document.getElementById('weightBody');

    if (weights.length === 0) {
        tbody.innerHTML = '<tr><td colspan="2" class="empty-state">No readings yet</td></tr>';
        return;
    }

    tbody.innerHTML = weights.map(w =>
        `<tr><td>${formatDateTime(w.created_at)}</td><td>${w.weight_kg.toFixed(2)}</td></tr>`
    ).join('');
}

function daysFromFilter(value) {
    if (value.startsWith('since:')) {
        const start = new Date(value.slice('since:'.length) + 'T00:00:00');
        const diffMs = Date.now() - start.getTime();
        const days = Math.ceil(diffMs / (1000 * 60 * 60 * 24));
        return days > 0 ? days : 1;
    }
    return parseInt(value, 10);
}

async function loadData() {
    const days = daysFromFilter(document.getElementById('daysFilter').value);
    try {
        const weights = await fetchWeights(days);
        renderChart(weights);
        renderSummary(weights);
        renderTable(weights);
    } catch (err) {
        console.error('Error loading weights:', err);
    }
}

document.getElementById('daysFilter').addEventListener('change', loadData);

// Initial load
loadData();
