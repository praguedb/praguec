using MessagePack;
using MongoDB.Bson.Serialization.Attributes;

namespace Common.IntegratedEventApi.Models.Branded;

/// <summary>
/// Reference to static data
/// </summary>
[MessagePackObject]
[BsonIgnoreExtraElements]
public class StaticDataRef : BaseProviderObject
{

}