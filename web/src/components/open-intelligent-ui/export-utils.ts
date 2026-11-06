// OpenIntelligentUI @ 8391a94; MIT, see web/licenses/OpenIntelligentUI.txt.
import {THEME_CSS} from './design-system';
import {CHART_COLORS} from './charts/config';
// Preserve text labels inside exported script elements.
const scriptJSON=(value:unknown)=>JSON.stringify(value).replaceAll('<','\\u003c').replaceAll('>','\\u003e').replaceAll('&','\\u0026');
export function chartToStandaloneHtml(
  type: "bar" | "pie",
  data: { title: string; description: string; data: Array<{ label: string; value: number }> }
): string {
  const labels = scriptJSON(data.data.map((d) => d.label));
  const values = JSON.stringify(data.data.map((d) => d.value));
  const colors = JSON.stringify(
    data.data.map((_, i) => CHART_COLORS[i % CHART_COLORS.length])
  );

  const chartConfig =
    type === "bar"
      ? `{
        type: 'bar',
        data: {
          labels: ${labels},
          datasets: [{
            data: ${values},
            backgroundColor: ${colors},
            borderRadius: 4,
          }]
        },
        options: {
          responsive: true,
          plugins: {
            legend: { display: false },
            tooltip: { backgroundColor: '#1f2937', titleColor: '#fff', bodyColor: '#fff', cornerRadius: 8, padding: 10 }
          },
          scales: {
            x: { grid: { display: false } },
            y: { grid: { color: 'rgba(0,0,0,0.06)' } }
          }
        }
      }`
      : `{
        type: 'pie',
        data: {
          labels: ${labels},
          datasets: [{
            data: ${values},
            backgroundColor: ${colors},
          }]
        },
        options: {
          responsive: true,
          plugins: {
            legend: { position: 'bottom', labels: { padding: 16, usePointStyle: true } },
            tooltip: { backgroundColor: '#1f2937', titleColor: '#fff', bodyColor: '#fff', cornerRadius: 8, padding: 10 }
          }
        }
      }`;

  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(data.title)}</title>
  <style>
    ${THEME_CSS}
    body { font-family: system-ui, -apple-system, sans-serif; padding: 24px; max-width: 640px; margin: 0 auto; }
    h3 { font-size: 20px; font-weight: 700; margin: 0 0 4px; color: var(--color-text-primary); }
    p { font-size: 14px; color: var(--color-text-secondary); margin: 0 0 20px; }
    canvas { max-height: 360px; }
  </style>
</head>
<body>
  <h3>${escapeHtml(data.title)}</h3>
  <p>${escapeHtml(data.description)}</p>
  <canvas id="chart"></canvas>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js"></script>
  <script>
    new Chart(document.getElementById('chart'), ${chartConfig});
  </script>
</body>
</html>`;
}

export function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function triggerDownload(htmlString: string, filename: string): void {
  const blob = new Blob([htmlString], { type: "text/html" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

