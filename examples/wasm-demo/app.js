(() => {
  "use strict";

  const left = document.getElementById("left");
  const right = document.getElementById("right");
  const product = document.getElementById("product");
  const status = document.getElementById("status");
  const multiplyButton = document.getElementById("multiply");
  const sampleButton = document.getElementById("sample");
  const swapButton = document.getElementById("swap");
  const copyButton = document.getElementById("copy");
  const limit = document.getElementById("limit");
  const multiplyTime = document.getElementById("multiply-time");
  const parseTime = document.getElementById("parse-time");
  const inputBits = document.getElementById("input-bits");
  const productSize = document.getElementById("product-size");

  function setStatus(message, state) {
    status.textContent = message;
    status.dataset.state = state;
  }

  function formatDuration(milliseconds) {
    if (milliseconds < 1) return `${(milliseconds * 1000).toFixed(0)} µs`;
    if (milliseconds < 1000) return `${milliseconds.toFixed(2)} ms`;
    return `${(milliseconds / 1000).toFixed(2)} s`;
  }

  function formatCount(value) {
    return new Intl.NumberFormat().format(value);
  }

  function loadLargeSample() {
    // 40,000 digits per operand is comfortably beyond bigfft's native FFT
    // crossover while remaining approachable for a browser demo.
    left.value = "314159265358979323846264338327950288419716939937510".repeat(800).slice(0, 40000);
    right.value = "271828182845904523536028747135266249775724709369995".repeat(800).slice(0, 40000);
    setStatus("FFT-sized sample loaded. Ready to multiply.", "ready");
    left.focus();
  }

  function showResult(result) {
    product.value = result.product;
    multiplyTime.textContent = formatDuration(result.multiplyMillis);
    parseTime.textContent = formatDuration(result.parseMillis);
    inputBits.textContent = `${formatCount(result.leftBits)} × ${formatCount(result.rightBits)}`;
    productSize.textContent = `${formatCount(result.productDigits)} digits · ${formatCount(result.productBits)} bits`;
    copyButton.disabled = false;
    setStatus(
      `Done — ${formatCount(result.leftDigits)} × ${formatCount(result.rightDigits)} decimal digits.`,
      "ready",
    );
  }

  function multiply() {
    if (!window.bigfft) return;

    multiplyButton.disabled = true;
    copyButton.disabled = true;
    setStatus("Multiplying…", "working");

    // Let the browser paint the working state before entering synchronous WASM.
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        try {
          const result = window.bigfft.multiply({ left: left.value, right: right.value });
          if (result.error) {
            setStatus(result.error, "error");
            return;
          }
          showResult(result);
        } catch (error) {
          console.error(error);
          setStatus(`WebAssembly call failed: ${error.message || error}`, "error");
        } finally {
          multiplyButton.disabled = false;
        }
      });
    });
  }

  async function copyResult() {
    try {
      await navigator.clipboard.writeText(product.value);
      copyButton.textContent = "Copied";
      setTimeout(() => {
        copyButton.textContent = "Copy result";
      }, 1200);
    } catch {
      product.select();
      setStatus("Select and copy the highlighted result.", "ready");
    }
  }

  async function boot() {
    const go = new Go();
    const response = await fetch("bigfft.wasm");
    if (!response.ok) throw new Error(`HTTP ${response.status} loading bigfft.wasm`);

    const bytes = await response.arrayBuffer();
    const result = await WebAssembly.instantiate(bytes, go.importObject);
    go.run(result.instance);

    await new Promise((resolve) => setTimeout(resolve, 0));
    if (!window.bigfft) throw new Error("the bigfft bridge did not initialize");

    limit.textContent = `Up to ${formatCount(window.bigfft.maxInputDigits)} digits per operand`;
    multiplyButton.disabled = false;
    setStatus("WebAssembly ready.", "ready");
  }

  multiplyButton.addEventListener("click", multiply);
  sampleButton.addEventListener("click", loadLargeSample);
  swapButton.addEventListener("click", () => {
    [left.value, right.value] = [right.value, left.value];
  });
  copyButton.addEventListener("click", copyResult);

  boot().catch((error) => {
    console.error(error);
    setStatus(`Could not load WebAssembly: ${error.message || error}`, "error");
  });
})();
