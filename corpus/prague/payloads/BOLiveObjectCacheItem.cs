using B2Tech.Feed.Cache.Core;
using Common.IntegratedEventApi.Models.LiveObject;
using MessagePack;

namespace BO.Services.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(BOLiveObjectCache))]
[DataCacheTopic("Cache.[v:brandId].LiveObject")]
public sealed partial class BOLiveObjectCacheItem
{
    [Key(0)]
    [DataCacheKey]
    public long EventId { get; set; }

    [Key(1)]
   
    public IBaseLiveData? LiveObject { get; set; }
}