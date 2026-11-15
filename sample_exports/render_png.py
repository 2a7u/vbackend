import pypdfium2 as pdfium
import os
import shutil

def render_page1_to_png(pdf_path, png_path, scale=2.5):
    pdf = pdfium.PdfDocument(pdf_path)
    page = pdf[0] # Page 1 (0-indexed)
    image = page.render(scale=scale).to_pil()
    image.save(png_path)
    print(f"Rendered {pdf_path} (page 1) -> {png_path} ({image.width}x{image.height})")

if __name__ == "__main__":
    sub_pdf = r"c:\Users\user\AndroidStudioProjects\UPOS\venus_backend\sample_exports\sample_subscriber_statement.pdf"
    rec_pdf = r"c:\Users\user\AndroidStudioProjects\UPOS\venus_backend\sample_exports\sample_reconciliation_statement.pdf"
    
    sub_png = r"c:\Users\user\AndroidStudioProjects\UPOS\venus_backend\sample_exports\page1_subscriber_statement.png"
    rec_png = r"c:\Users\user\AndroidStudioProjects\UPOS\venus_backend\sample_exports\page1_reconciliation_statement.png"
    
    render_page1_to_png(sub_pdf, sub_png)
    render_page1_to_png(rec_pdf, rec_png)
    
    # Copy both PDFs and rendered PNGs to the artifacts folder
    art_dir = r"C:\Users\user\.gemini\antigravity\brain\04e8d6a2-9d33-4ddb-8ce6-1af6ba6f0f14"
    for src in [sub_pdf, rec_pdf, sub_png, rec_png]:
        dst = os.path.join(art_dir, os.path.basename(src))
        shutil.copy2(src, dst)
        print(f"Copied to artifact dir: {dst} ({os.path.getsize(dst)} bytes)")
