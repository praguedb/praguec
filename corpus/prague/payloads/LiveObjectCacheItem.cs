using B2Tech.Feed.Cache.Core;
using Common.IntegratedEventApi.Models.LiveObject;
using MessagePack;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(LiveObjectCache))]
[DataCacheTopic("Cache.[v:brandId].LiveObject")]
public sealed partial class LiveObjectCacheItem
{
    [Key(0)]
    [DataCacheKey]
    public long EventId { get; set; }

    [Key(1)]
    public IBaseLiveData? LiveObject { get; set; }
}
