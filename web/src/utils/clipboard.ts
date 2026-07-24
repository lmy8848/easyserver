import { message } from 'antd';

export const copyToClipboard = (text: string, successMsg = '已复制') => {
  const doFallback = () => {
    // 方案 2：优雅的 span 选区方式
    // 与 textarea 不同，通过 span 和 Selection API 选中文本不会导致 DOM 焦点（focus）发生转移。
    // 因此能够完美绕过所有的 Modal / Dialog 焦点陷阱，不需要去查找具体的容器。
    const span = document.createElement('span');
    span.textContent = text;
    span.style.all = 'unset';
    span.style.position = 'fixed';
    span.style.top = '0';
    span.style.clip = 'rect(0, 0, 0, 0)';
    span.style.whiteSpace = 'pre';
    span.style.webkitUserSelect = 'text';
    span.style.userSelect = 'text';
    
    document.body.appendChild(span);

    const selection = window.getSelection();
    let originalRange: Range | null = null;
    if (selection) {
      originalRange = selection.rangeCount > 0 ? selection.getRangeAt(0) : null;
      selection.removeAllRanges();
      const range = document.createRange();
      range.selectNodeContents(span);
      selection.addRange(range);
    }

    try {
      const successful = document.execCommand('copy');
      if (successful) {
        message.success(successMsg);
      } else {
        message.error('复制失败，请检查浏览器权限');
      }
    } catch (e) {
      message.error('复制失败，请手动复制');
    }

    document.body.removeChild(span);
    if (selection) {
      selection.removeAllRanges();
      if (originalRange) {
        selection.addRange(originalRange);
      }
    }
  };

  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text)
      .then(() => message.success(successMsg))
      .catch(doFallback);
  } else {
    doFallback();
  }
};
