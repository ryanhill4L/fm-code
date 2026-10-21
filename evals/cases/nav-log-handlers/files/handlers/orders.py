def handle_create_order(request):
    return {"order": request.get("item")}


def handle_cancel_order(request):
    return {"cancelled": request.get("id")}
