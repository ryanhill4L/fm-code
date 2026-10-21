def resize(img, width, height, keep_ratio=True):
    if keep_ratio:
        height = int(width * img["h"] / img["w"])
    return {"w": width, "h": height}
