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

    // Chart data is chronological (oldest first)
    const sorted = [...weights].reverse();
    const labels = sorted.map(w => formatDate(w.created_at));
    const data = sorted.map(w => w.weight_kg);

    if (chart) {
        chart.destroy();
    }

    chart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
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
                        label: function(ctx) {
                            return ctx.parsed.y.toFixed(2) + ' kg';
                        }
                    }
                }
            },
            scales: {
                x: {
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

async function loadData() {
    const days = parseInt(document.getElementById('daysFilter').value, 10);
    try {
        const weights = await fetchWeights(days);
        renderChart(weights);
        renderTable(weights);
    } catch (err) {
        console.error('Error loading weights:', err);
    }
}

document.getElementById('daysFilter').addEventListener('change', loadData);

// Initial load
loadData();
