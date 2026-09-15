from sources.schemas import QueryPreview, SearchInput


class SourceService:
    def preview(self, data: SearchInput) -> QueryPreview:
        return QueryPreview(
            query=data.keyword, since=data.since, until=data.until, limit=data.limit
        )
